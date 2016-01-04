package log

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"time"
)

var Stream io.Writer = os.Stderr
var prev io.Writer // value of Stream in previous log() call

var lineRE *regexp.Regexp = regexp.MustCompile("(?m)^")

func log1(line string) {
	var timestamp string
	if prev != Stream {
		/* prefix first message with time zone */
		timestamp = time.Now().Format("2006-01-02T15:04:05.000Z07:00")
		prev = Stream
	} else {
		timestamp = time.Now().Format("2006-01-02T15:04:05.000")
	}
	if len(line) > 0 && line[len(line) - 1] == '\n' {
		line = line[:len(line) - 1]
	}
	fmt.Fprintln(Stream, timestamp + " " + line)
}

func log(prefix, msg string) {
	for _, line := range lineRE.Split(msg, -1) {
		log1(prefix + line)
	}
}

type LogContext struct {
	Prefix string
}

func (l LogContext) Println(args... interface{}) {
	log(l.Prefix, fmt.Sprintln(args...))
}

func (l LogContext) Printf(format string, args... interface{}) {
	log(l.Prefix, fmt.Sprintf(format, args...))
}

var DB = LogContext{"   DB "}
var FS = LogContext{"FILES "}
var AU = LogContext{"AUDIO "}
var WAV = LogContext{" WAVE "}
var UI = LogContext{"   UI "}

func Printf(format string, args... interface{}) {
	LogContext{}.Printf(format, args...)
}

func Println(args... interface{}) {
	LogContext{}.Println(args...)
}
