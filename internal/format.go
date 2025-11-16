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

type FormatService struct {
	Client string
}

func NewFormatService(client string) *FormatService {
	if client == "" {
		client = "VLoggo"
	}

	return &FormatService{
		Client: client,
	}
}

func (fs *FormatService) Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

func (fs *FormatService) IsoDate(t ...time.Time) string {
	date := time.Now()

	if len(t) > 0 {
		date = t[0]
	}
	return date.UTC().Format(time.RFC3339)
}

func (fs *FormatService) Filename() string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	return "log-" + dateStr + ".txt"
}

func (fs *FormatService) JSONFilename() string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	return fmt.Sprintf("log-%s.jsonl", dateStr)
}

func (fs *FormatService) Line(entry types.LogEntry) string {
	timestamp := fs.Date()

	return fmt.Sprintf("[%s] [%s] [%s] [%s] [%s] : %s\n",
		fs.Client,
		timestamp,
		entry.Level,
		entry.Code,
		entry.Caller,
		entry.Message,
	)
}

func (fs *FormatService) JSONLine(entry types.LogEntry) string {
	jsonEntry := struct {
		Client    string `json:"client"`
		Timestamp string `json:"timestamp"`
		types.LogEntry
	}{
		Client:    fs.Client,
		Timestamp: fs.IsoDate(),
		LogEntry:  entry,
	}

	jsonBytes, err := json.Marshal(jsonEntry)
	if err != nil {
		return fmt.Sprintf("[VLoggo] > [%s] [%s] [ERROR] : failed to serialize log > %v",
			fs.Client,
			fs.Date(),
			err,
		)
	}

	return string(jsonBytes) + "\n"
}

func (fs *FormatService) Separator() string {
	separator := "\n" + strings.Repeat("_", 50) + "\n\n"
	timestamp := fs.Date()

	return fmt.Sprintf("%s[%s] [%s] [INIT] : VLoggo initialized successfully \n",
		separator,
		fs.Client,
		timestamp,
	)
}

func (fs *FormatService) JSONSeparator() string {
	initEntry := struct {
		Client    string `json:"client"`
		Timestamp string `json:"timestamp"`
		Level     string `json:"level"`
		Message   string `json:"message"`
	}{
		Client:    fs.Client,
		Timestamp: fs.IsoDate(),
		Level:     "INIT",
		Message:   "VLoggo initialized successfully",
	}

	jsonBytes, err := json.Marshal(initEntry)
	if err != nil {

		return fmt.Sprintf("[VLoggo] > [%s] [%s] [ERROR] : failed to serialize init log > %v",
			fs.Client,
			fs.Date(),
			err,
		)
	}
	return string(jsonBytes) + "\n"
}

func Caller(skip int) string {

	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "(unknown:0)"
	}

	parts := strings.Split(file, "/")
	filename := parts[len(parts)-1]

	return fmt.Sprintf("%s:%s", filename, strconv.Itoa(line))
}
