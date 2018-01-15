package gossh

import (
	"fmt"
	"time"

	"github.com/golang/glog"
)

type Logger struct {
	start      time.Time
	subject    string
	debugLevel glog.Level
}

func GetLogger(subject string, debugLevel int32) *Logger {
	return &Logger{time.Now(), subject, glog.Level(debugLevel)}
}

func (l *Logger) WithSubject(subject string) *Logger {
	return &Logger{l.start, subject, l.debugLevel}
}

func (l *Logger) prefix() string {
	elapsed := time.Since(l.start).Round(time.Microsecond * 10)
	return fmt.Sprintf("%s - %s] ", elapsed, l.subject)
}

func (l *Logger) Debug(args ...interface{}) {
	args = append([]interface{}{l.prefix()}, args...)
	glog.V(l.debugLevel).Info(args...)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	format = l.prefix() + format
	glog.V(l.debugLevel).Infof(format, args...)
}
