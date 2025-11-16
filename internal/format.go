// Package services provides formatting and file management services for VLoggo.
// Includes FormatService for log formatting and timestamps, and FileService for file operations,
// log rotation and retention management.
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

// FormatService manages log entry formatting, filenames and timestamps
type FormatService struct {
	Client string
}

// NewFormatService creates a new FormatService instance
// If client is empty, defaults to "VLoggo"
func NewFormatService(client string) *FormatService {
	if client == "" {
		client = "VLoggo"
	}
	return &FormatService{
		Client: client,
	}
}

// Date formats a timestamp in Brazilian format (DD/MM/YYYY HH:MM:SS)
// If no time is provided, uses current time
func (fs *FormatService) Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

// IsoDate formats a timestamp in ISO 8601 / RFC3339 format (UTC)
// If no time is provided, uses current time
func (fs *FormatService) IsoDate(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.UTC().Format(time.RFC3339)
}

// Filename generates the log filename based on current date
// Format: log-YYYY-MM-DD.txt
func (fs *FormatService) Filename() string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	return "log-" + dateStr + ".txt"
}

// JSONFilename generates the JSON log filename based on current date
// Format: log-YYYY-MM-DD.jsonl
func (fs *FormatService) JSONFilename() string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	return fmt.Sprintf("log-%s.jsonl", dateStr)
}

// Line formats a log entry into a human-readable text line
// Format: [Client] [Timestamp] [Level] [Code] [Caller] : Message
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

// JSONLine formats a log entry in JSON Lines (JSONL) format
// Each entry is a complete JSON object followed by a newline
// Returns an error message if serialization fails
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

// Separator generates a visual separator for log files with initialization message
// Used when creating a new log file to mark the start
func (fs *FormatService) Separator() string {
	separator := "\n" + strings.Repeat("_", 50) + "\n\n"
	timestamp := fs.Date()
	return fmt.Sprintf("%s[%s] [%s] [INIT] : VLoggo initialized successfully \n",
		separator,
		fs.Client,
		timestamp,
	)
}

// JSONSeparator generates a JSON initialization entry for JSON log files
// Used when creating a new JSON log file to mark the start
// Returns an error message if serialization fails
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

// Caller gets caller information (filename and line number) from the call stack
// Used to identify where a log entry originated in the code
// skip defines how many stack frames to skip (typically 3 for log methods)
// Returns "(unknown:0)" if information is unavailable
func Caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "(unknown:0)"
	}
	parts := strings.Split(file, "/")
	filename := parts[len(parts)-1]
	return fmt.Sprintf("%s:%s", filename, strconv.Itoa(line))
}