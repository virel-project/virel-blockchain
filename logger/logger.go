package logger

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var DiscardLog = &Log{
	logLevel: 0,
	stdout:   io.Discard,
	stderr:   io.Discard,
}

func New() *Log {
	return &Log{
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		logLevel: 1,
	}
}

type Log struct {
	logLevel uint8
	stdout   io.Writer
	stderr   io.Writer
	sync.RWMutex
}

func (l *Log) SetLogLevel(lvl uint8) {
	l.Lock()
	defer l.Unlock()

	l.logLevel = lvl
}
func (l *Log) GetLogLevel() uint8 {
	l.RLock()
	defer l.RUnlock()

	return l.logLevel
}
func (l *Log) SetStdout(stdout io.Writer) {
	l.Lock()
	defer l.Unlock()

	l.stdout = stdout
}
func (l *Log) SetStderr(stderr io.Writer) {
	l.Lock()
	defer l.Unlock()

	l.stderr = stderr
}

var Reset = "\033[0m"
var Red = "\033[31m"
var Green = "\033[32m"
var Yellow = "\033[33m"
var Blue = "\033[34m"
var Purple = "\033[35m"
var Cyan = "\033[36m"
var Gray = "\033[37m"
var White = "\033[97m"
var Bold = "\033[1m"

func getLogPrefix() string {
	_, file, line, _ := runtime.Caller(2)
	fileSpl := strings.Split(file, "/")
	debugInfos := strings.Split(fileSpl[len(fileSpl)-1], ".")[0] + ":" + strconv.FormatInt(int64(line), 10)
	for len(debugInfos) < 18 {
		debugInfos = debugInfos + " "
	}

	return getTime() + debugInfos
}
func getTime() string {
	t := time.Now()
	s := fmt.Sprintf("%02d:%02d:%02d.%03d", t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1000/1000)
	return s + " "
}

func (l *Log) Info(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + "I " + fmt.Sprintln(a...) + Reset))
}
func (l *Log) Infof(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + fmt.Sprintf("I "+format+"\n", a...) + Reset))
}

func (l *Log) Warn(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Yellow + "W " + fmt.Sprintln(a...) + Reset))
}
func (l *Log) Warnf(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Yellow + fmt.Sprintf("W "+format+"\n", a...) + Reset))
}

func (l *Log) Err(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Red + "E " + fmt.Sprintln(a...) + Reset))
}

func (l *Log) Errf(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stderr.Write([]byte(getLogPrefix() + Red + fmt.Sprintf("E "+format+"\n", a...) + Reset))
}

func (l *Log) Debug(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 2 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Cyan + "D " + fmt.Sprintln(a...) + Reset))
}
func (l *Log) Debugf(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 2 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Cyan + fmt.Sprintf("D "+format+"\n", a...) + Reset))
}

func (l *Log) Dev(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 3 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Cyan + "d " + fmt.Sprintln(a...) + Reset))
}

func (l *Log) Devf(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 3 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Cyan + fmt.Sprintf("d "+format+"\n", a...) + Reset))
}

func (l *Log) Net(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 3 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Green + "N " + fmt.Sprintln(a...) + Reset))
}
func (l *Log) Netf(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 3 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Green + fmt.Sprintf("N "+format+"\n", a...) + Reset))
}

func (l *Log) NetDev(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 4 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Green + "n " + fmt.Sprintln(a...) + Reset))
}

func (l *Log) NetDevf(format string, a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 4 {
		return
	}
	l.stdout.Write([]byte(getLogPrefix() + Green + fmt.Sprintf("n "+format+"\n", a...) + Reset))
}

func (l *Log) Fatal(a ...any) {
	l.Lock()
	defer l.Unlock()
	if l.logLevel < 1 {
		return
	}
	l.stderr.Write([]byte(getLogPrefix() + Red + "F " + fmt.Sprintln(a...) + Reset))
	panic(fmt.Sprintln(a...))
}
