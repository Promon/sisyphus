package jobmon

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"regexp"
	k "sisyphus/kubernetes"
	"strings"
	"time"
)

type LogLine struct {
	timestamp time.Time
	text      string
}

type LogState struct {
	lastLogLineTimestamp *time.Time

	// Memory of previous lines
	previousLineHash  []uint64
	localLogger       *logrus.Entry
	logBuffer         *bytes.Buffer
	gitlabStartOffset int
	lineBreakRegexp   *regexp.Regexp
}

const (
	LogFetchTimeout        = 10 * time.Second
	PreviousLineMemorySize = 10240
)

func (ls *LogState) bufferLogs(job *k.Job) error {
	// Fetch logs with timeout
	chChunk := make(chan *bytes.Buffer, 1)
	chErr := make(chan error, 1)

	go func() {
		rdr, err := job.GetLog(ls.lastLogLineTimestamp)
		if err != nil {
			chErr <- err
			return
		}
		chChunk <- rdr
	}()

	select {
	case chunk := <-chChunk:
		err := ls.printLog(chunk)
		return err

	case err := <-chErr:
		return err

	case <-time.After(LogFetchTimeout):
		return errors.New("fetching of logs from k8s timed out")
	}
}

func (ls *LogState) printLog(logChunk *bytes.Buffer) error {

	// Filter trimLines
	tmpLines := ls.lineBreakRegexp.Split(logChunk.String(), -1)
	trimLines := make([]string, 0, len(tmpLines))
	for _, l := range tmpLines {
		trimmed := strings.TrimSpace(l)
		if len(trimmed) > 0 {
			trimLines = append(trimLines, trimmed)
		}
	}

	// Parse trimLines
	parsedLines := parseLogLines(trimLines)

	// filter already printed lines
	var filteredLines []LogLine
	if ls.lastLogLineTimestamp != nil {
		filteredLines = keepLinesAfter(parsedLines, *ls.lastLogLineTimestamp)
	} else {
		filteredLines = parsedLines
	}

	// Remember last timestamp
	if len(filteredLines) > 0 {
		ls.lastLogLineTimestamp = &filteredLines[len(filteredLines)-1].timestamp
	}

	// print lines to gitlab buffer
	for _, l := range filteredLines {
		_, err := fmt.Fprintln(ls.logBuffer, l.text)
		if err != nil {
			return err
		}
	}

	return nil
}

func newLogState(localLogger *logrus.Entry) *LogState {
	var logBuff bytes.Buffer
	return &LogState{
		lastLogLineTimestamp: nil,
		logBuffer:            &logBuff,
		gitlabStartOffset:    0,
		localLogger:          localLogger,
		previousLineHash:     make([]uint64, 0, PreviousLineMemorySize),
		lineBreakRegexp:      regexp.MustCompile("\r?\n"),
	}
}

// Keep only lines newer than a timestamp
func keepLinesAfter(lines []LogLine, minTime time.Time) []LogLine {
	for threshold, p := range lines {
		if p.timestamp.After(minTime) {
			return lines[threshold:]
		}
	}

	return nil
}

// Parse timestamps in multiple log lines
func parseLogLines(rawLines []string) []LogLine {
	if len(rawLines) == 0 {
		return nil
	}

	result := make([]LogLine, 0, len(rawLines))

	for _, raw := range rawLines {
		p, err := parseLogLine(raw)
		if err != nil {
			continue
		} else {
			result = append(result, *p)
		}
	}

	return result
}

// Split log line to timestamp and text
func parseLogLine(logLine string) (*LogLine, error) {
	parts := strings.SplitN(logLine, " ", 2)
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}

	var lineTxt = ""
	if len(parts) > 1 {
		lineTxt = parts[1]
	}

	return &LogLine{
		timestamp: ts,
		text:      lineTxt,
	}, nil
}
