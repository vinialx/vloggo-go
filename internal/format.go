package services

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	types "github.com/vinialx/vloggo-go/types"
)

type Formatter struct {
	Client string
}

func NewFormatter(client string) *Formatter {
	if client == "" {
		client = "VLoggo"
	}

	return &Formatter{
		Client: client,
	}
}

func (f *Formatter) Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

func (f *Formatter) IsoDate(t ...time.Time) string {
	date := time.Now()

	if len(t) > 0 {
		date = t[0]
	}
	return date.UTC().Format(time.RFC3339)
}

func (f *Formatter) Filename() string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	return "log-" + dateStr + ".txt"
}

func (f *Formatter) JSONFilename() string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	return fmt.Sprintf("log-%s.jsonl", dateStr)
}

func (f *Formatter) Line(entry types.LogEntry) string {
	timestamp := f.Date()

	return fmt.Sprintf("[%s] [%s] [%s] [%d] [%s] : %s\n",
		f.Client,
		timestamp,
		entry.Level,
		entry.Code,
		entry.Caller,
		entry.Message,
	)
}

func (f *Formatter) JSONLine(entry types.LogEntry, pretty bool) string {
	jsonEntry := struct {
		Client    string `json:"client"`
		Timestamp string `json:"timestamp"`
		types.LogEntry
	}{
		Client:    f.Client,
		Timestamp: f.IsoDate(),
		LogEntry:  entry,
	}

	jsonBytes, err := json.Marshal(jsonEntry)
	if err != nil {
		return fmt.Sprintf("[VLoggo] > [%s] [%s] [ERROR] : failed to serialize log > %v",
			f.Client,
			f.Date(),
			err,
		)
	}

	return string(jsonBytes) + "\n"
}

func (f *Formatter) Separator() string {
	separator := "\n" + strings.Repeat("_", 50) + "\n\n"
	timestamp := f.Date()

	return fmt.Sprintf("%s[%s] [%s] [INIT] : VLoggo initialized successfully \n",
		separator,
		f.Client,
		timestamp,
	)
}

func (f *Formatter) JSONSeparator() string {
	initEntry := struct {
		Client    string `json:"client"`
		Timestamp string `json:"timestamp"`
		Level     string `json:"level"`
		Message   string `json:"message"`
	}{
		Client:    f.Client,
		Timestamp: f.IsoDate(),
		Level:     "INIT",
		Message:   "VLoggo initialized successfully",
	}

	jsonBytes, err := json.Marshal(initEntry)
	if err != nil {

		return fmt.Sprintf("[VLoggo] > [%s] [%s] [ERROR] : failed to serialize init log > %v",
			f.Client,
			f.Date(),
			err,
		)
	}
	return string(jsonBytes) + "\n"
}

func (f *Formatter) Caller(skip int) string {

	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "(unknown:0)"
	}

	parts := strings.Split(file, "/")
	filename := parts[len(parts)-1]

	return fmt.Sprintf("%s:%s", filename, strconv.Itoa(line))
}
