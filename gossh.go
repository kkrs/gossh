package gossh

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type OutputHandler func(host string, stream io.Reader)
type StatusHandler func(host string, err error)

// mutex on calling stdout
var stdoutMutex sync.Mutex

const DefaultDebugLevel = 2

func printLines(host, stream string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		stdoutMutex.Lock()
		fmt.Printf("%s:%s:%s\n", host, stream, scanner.Text())
		stdoutMutex.Unlock()
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "gossh.printLines(%s, %s): %s\n", host, stream, err)
	}
}

func PrintStdout(endpoint string, stream io.Reader) {
	printLines(endpoint, "stdout", stream)
}

func PrintStderr(endpoint string, stream io.Reader) {
	printLines(endpoint, "stderr", stream)
}

func PrintStatus(endpoint string, status error) {
	stdoutMutex.Lock()
	defer stdoutMutex.Unlock()
	if status != nil {
		switch status := status.(type) {
		case *ssh.ExitError:
			fmt.Printf("%s:status:failure, exit status: %d\n", endpoint, status.ExitStatus())
		default:
			fmt.Printf("%s:status:failure, error: %s\n", endpoint, status)
		}
	} else {
		fmt.Printf("%s:status:success, exit status: 0\n", endpoint)
	}
}

// IdentityFile converts ssh key file to ssh.AuthMethod. Encrypted IdentityFiles are not handled for
// now.
func IdentityFile(file string) (ssh.AuthMethod, error) {
	pem, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("IdentityFile: %s", err)
	}

	key, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		return nil, fmt.Errorf("IdentityFile: %s", err)
	}

	return ssh.PublicKeys(key), nil
}

func AgentKeys(sockPath string) (ssh.AuthMethod, error) {
	sock, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, err
	}
	a := agent.NewClient(sock)
	signers, err := a.Signers()
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signers...), nil
}

type Config struct {
	Login          string
	AuthMethods    []ssh.AuthMethod
	ConnectTimeout time.Duration
	SessionTimeout time.Duration
	StdoutHandler  OutputHandler
	StderrHandler  OutputHandler
	StatusHandler  StatusHandler
	Logger         *Logger
}

func RunOn(hosts []string, cmd string, maxFlight int, cfg *Config) {
	if maxFlight < 1 {
		maxFlight = 1
	}

	if cfg.Logger == nil {
		cfg.Logger = GetLogger("main", 2)
	}

	workers := new(sync.WaitGroup)
	sem := make(chan struct{}, maxFlight)
	defer close(sem)

	for _, host := range hosts {
		sem <- struct{}{}
		workers.Add(1)
		go func(host string) {
			defer func() {
				workers.Done()
				<-sem
			}()
			Run(host, cmd, cfg)
		}(host)
	}
	workers.Wait()
}

func Run(host, cmd string, cfg *Config) error {
	client, err := Dial(host, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Run(cmd)
}

type Client struct {
	logger         *Logger
	client         *ssh.Client
	host           string
	sessionTimeout time.Duration
	handleStdout   OutputHandler
	handleStderr   OutputHandler
	handleStatus   StatusHandler
}

func Dial(host string, cfg *Config) (*Client, error) {
	if cfg.Logger == nil {
		cfg.Logger = GetLogger(host, 2)
	} else {
		cfg.Logger = cfg.Logger.WithSubject(host)
	}

	client, err := dial(host, cfg)
	if err != nil && cfg.StatusHandler != nil {
		cfg.StatusHandler(host, err)
		return nil, err
	}
	return client, nil
}

func toAddr(host string) string {
	if strings.Index(host, ":") == -1 {
		return host + ":22"
	}
	return host
}

func dial(host string, cfg *Config) (*Client, error) {
	lgr := cfg.Logger
	lgr.Debugf("connecting")
	addr := toAddr(host)
	conn, err := net.DialTimeout("tcp", addr, cfg.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	lgr.Debug("connected, setting up ssh conn")
	clientConfig := &ssh.ClientConfig{
		User:            cfg.Login,
		Auth:            cfg.AuthMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		// this doesn't work if the remote does not respond after a tcp conn is established.
		Timeout: cfg.ConnectTimeout,
	}
	lgr.Debugf("ConnectTimeout: %s", clientConfig.Timeout)

	// clientConfig.Timeout does not work when the tcp connection is established but the remote
	// ssh process does not respond after that.
	conn.SetReadDeadline(time.Now().Add(cfg.ConnectTimeout))
	sshConn, newChannel, newRequest, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		return nil, err
	}
	// reset
	conn.SetReadDeadline(time.Time{})
	lgr.Debug("done")

	return &Client{
		lgr,
		ssh.NewClient(sshConn, newChannel, newRequest),
		host,
		cfg.SessionTimeout,
		cfg.StdoutHandler,
		cfg.StderrHandler,
		cfg.StatusHandler,
	}, nil
}

// close to release all sessions associated with the conn and maybe blocked
func (cl *Client) Close() error {
	return cl.client.Close() // cl.client can never be nil
}

type SessionTimeoutError struct{}

func (e *SessionTimeoutError) Error() string {
	return "session timeout"
}

func (cl *Client) Run(cmd string) error {
	err := cl.run(cmd)
	if cl.handleStatus != nil {
		cl.handleStatus(cl.host, err)
	}
	return err
}

// TODO: fix comment
// can goroutines & session when timeout occurs and remote command has not exited. Session.Close()
// does not shutdown the session stdout,stderr pipes immediately and goroutines that service them
// will hang around till they close. session.Wait() will hang around till remote command exists.
// Closing the underlying connection will close all sessions on that conn.
func (cl *Client) run(cmd string) error {
	sess, err := cl.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	cl.logger.Debug("session opened")

	var stdout, stderr io.Reader

	if cl.handleStdout != nil {
		stdout, err = sess.StdoutPipe()
		if err != nil {
			return err
		}
	}

	if cl.handleStderr != nil {
		stderr, err = sess.StderrPipe()
		if err != nil {
			return err
		}
	}

	if stdout != nil {
		go func() {
			cl.handleStdout(cl.host, stdout)
		}()
	}

	if stderr != nil {
		go func() {
			cl.handleStderr(cl.host, stderr)
		}()
	}

	cl.logger.Debug("command started")
	if err := sess.Start(cmd); err != nil {
		return err
	}

	var d time.Duration = cl.sessionTimeout
	if d == 0 {
		d = time.Duration(math.MaxInt64)
	}
	timeout := time.NewTimer(d)
	status := make(chan error, 1)

	go func() {
		status <- sess.Wait()
	}()

	var exit error

	// block until timeout or we get status
	select {
	case <-timeout.C:
		cl.logger.Debug("command timed out")
		sess.Signal(ssh.SIGQUIT) // attempt to signal the other end
		sess.Close()
		exit = &SessionTimeoutError{}
	case exit = <-status:
		cl.logger.Debug("command exited")
		timeout.Stop()
	}

	return exit
}
