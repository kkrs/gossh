package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kkrs/gossh"
	"golang.org/x/crypto/ssh"
)

func print(prefix string, v ...interface{}) {
	msg := fmt.Sprint(v...)
	if msg[len(msg)-1] != '\n' {
		msg += "\n"
	}
	fmt.Fprint(os.Stderr, prefix, msg)
}

func printf(prefix, format string, args ...interface{}) {
	print(prefix, fmt.Sprintf(format, args...))
}

func usage(format string, args ...interface{}) {
	printf("usage: ", format, args...)
	os.Exit(255)
}

func fatal(v ...interface{}) {
	print("fatal: ", v...)
	os.Exit(255)
}

func fatalf(format string, args ...interface{}) {
	fatal(fmt.Sprintf(format, args...))
}

func warn(v ...interface{}) {
	print("warn: ", v...)
}

func warnf(format string, args ...interface{}) {
	warn(fmt.Sprintf(format, args...))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	// return true when err is not nil and not IsNotExist as well as when err is nil
	return true
}

func identityFiles(methods []ssh.AuthMethod, files ...string) []ssh.AuthMethod {
	for _, k := range files {
		m, err := gossh.IdentityFile(k)
		if err != nil {
			warn(err)
			continue
		}
		methods = append(methods, m)
	}
	return methods
}

func agentKeys(methods []ssh.AuthMethod, authSock string) []ssh.AuthMethod {
	if exists(authSock) {
		agentMethod, err := gossh.AgentKeys(authSock)
		if err != nil {
			warn(err)
		} else {
			methods = append(methods, agentMethod)
		}
	}
	return methods
}

var (
	succeeded []string
	failed    []string
	mu        sync.Mutex
	Version   string
)

func printAndCollateStatus(host string, status error) {
	gossh.PrintStatus(host, status)

	mu.Lock()
	defer mu.Unlock()
	if status != nil {
		failed = append(failed, host)
		return
	}
	succeeded = append(succeeded, host)
}

func readFileOrStdin(file string) ([]byte, error) {
	if file == "-" {
		return ioutil.ReadAll(os.Stdin)
	} else {
		return ioutil.ReadFile(file)
	}
}

func targets(rangeExpr, hostsFile string) ([]string, error) {
	if rangeExpr != "" {
		hosts, err := expand(rangeExpr)
		return hosts, err
	}

	if hostsFile != "" {
		data, err := readFileOrStdin(hostsFile)
		if err != nil {
			return nil, err
		}
		return strings.Fields(string(data)), nil
	}

	return nil, errors.New("missing argument, one of range, hostsFile required")
}

func main() {
	var (
		identities     string
		useAgent       bool
		login          string
		port           string
		rangeExpr      string
		hostsFile      string
		maxFlight      int
		connTimeout    float64
		sessionTimeout float64
		displayVersion bool
	)
	authSock := os.Getenv("SSH_AUTH_SOCK")

	flag.StringVar(&identities, "i", "", "identity files to use")
	useAgentMsg := fmt.Sprintf("use agent defined at $SSH_AUTH_SOCK(%s)", authSock)
	flag.BoolVar(&useAgent, "G", true, useAgentMsg)
	flag.StringVar(&login, "l", os.Getenv("LOGNAME"), "login name")
	flag.StringVar(&port, "p", "22", "port to connect on")
	flag.StringVar(&rangeExpr, "r", "", "range to run command on")
	flag.StringVar(&hostsFile, "H", "", "file containing hosts to run the command on")
	flag.IntVar(&maxFlight, "m", 100, "maximum number of connections in flight")
	flag.Float64Var(&connTimeout, "c", 10.0, "connection timeout in seconds, 0 for none")
	flag.Float64Var(&sessionTimeout, "t", 0, "session timeout in seconds, 0 for none")
	flag.BoolVar(&displayVersion, "version", false, "version")
	flag.Parse()

	cmd := strings.Join(flag.Args(), " ")

	if displayVersion {
		fmt.Printf("%s %s\n", os.Args[0], Version)
		os.Exit(0)
	}

	hosts, err := targets(rangeExpr, hostsFile)
	if err != nil {
		fatal(err)
	}

	var methods []ssh.AuthMethod
	if identities != "" {
		methods = identityFiles(methods, identities)
	}

	if useAgent {
		methods = agentKeys(methods, authSock)
	}

	if len(methods) == 0 {
		fatal("no authentication methods remain")
	}

	cfg := &gossh.Config{
		login,
		methods,
		time.Millisecond * time.Duration(connTimeout*1000),
		time.Millisecond * time.Duration(sessionTimeout*1000),
		gossh.PrintStdout,
		gossh.PrintStderr,
		printAndCollateStatus,
		gossh.GetLogger("main", 2),
	}

	gossh.RunOn(hosts, cmd, maxFlight, cfg)
	fmt.Println()
	fmt.Printf("succeeded: %s\n", compress(succeeded))
	fmt.Printf("failed: %s\n", compress(failed))
}
