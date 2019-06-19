package jobmon

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	k "sisyphus/kubernetes"
	"strings"
	"time"
)

type LogLine struct {
	timestamp time.Time
	text      string
}

type LogState struct {
	lastSeenLine      *LogLine
	localLogger       *logrus.Entry
	logBuffer         *bytes.Buffer
	gitlabStartOffset int
}

func (ls *LogState) bufferLogs(job *k.Job) error {
	var sinceTime *time.Time = nil

	if ls.lastSeenLine != nil {
		sinceTime = &ls.lastSeenLine.timestamp
	}

	rdr, err := job.GetLog(sinceTime)
	if err != nil {
		return err
	}
	defer rdr.Close()

	return ls.printLog(rdr)
}

func (ls *LogState) printLog(log io.ReadCloser) error {
	sc := bufio.NewScanner(log)
	for sc.Scan() {
		timestamped := sc.Text()
		parsed, err := parseLogLine(timestamped)

		if err != nil {
			ls.localLogger.Warnf("Invalid log line: `%s`", timestamped)
			continue
		}

		// skip lines older than last seen
		if ls.lastSeenLine == nil || parsed.timestamp.After(ls.lastSeenLine.timestamp) {
			// remember last line we seen
			ls.lastSeenLine = parsed
			//fmt.Println(timestamped)
			fmt.Fprintln(ls.logBuffer, parsed.text)
		}
	}

	return nil
}

func newLogState(localLogger *logrus.Entry) *LogState {
	var logBuff bytes.Buffer
	return &LogState{
		logBuffer:         &logBuff,
		gitlabStartOffset: 0,
		localLogger:       localLogger,
		lastSeenLine:      nil,
	}
}

// Split log line to timestamp and text
func parseLogLine(logLine string) (*LogLine, error) {
	parts := strings.SplitN(logLine, " ", 2)
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}

	return &LogLine{
		timestamp: ts,
		text:      parts[1],
	}, nil
}
