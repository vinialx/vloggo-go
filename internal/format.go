// Package services provides formatting, redaction, file, and SMTP services for VLoggo.
package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vinialx/vloggo-go/v2/types"
)

type FormatService struct {
	Client   string
	Timezone string
	Location *time.Location
}

func NewFormatService(client string, timezone ...string) *FormatService {
	if client == "" {
		client = "VLoggo"
	}
	tz := time.Local.String()
	if len(timezone) > 0 && timezone[0] != "" {
		tz = timezone[0]
	}
	location, err := time.LoadLocation(tz)
	if err != nil {
		location, tz = time.Local, time.Local.String()
	}
	return &FormatService{Client: client, Timezone: tz, Location: location}
}

func (fs *FormatService) SetTimezone(timezone string) error {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return err
	}
	fs.Timezone, fs.Location = timezone, location
	return nil
}

func NewEventID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		now := uint64(time.Now().UnixNano())
		for index := range id {
			id[index] = byte(now >> ((index % 8) * 8))
		}
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func (fs *FormatService) ShortID(eventID string) string {
	compact := strings.ReplaceAll(strings.ToLower(eventID), "-", "")
	if len(compact) > 8 {
		compact = compact[:8]
	}
	return "evt-" + compact
}

func (fs *FormatService) Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	location := fs.Location
	if location == nil {
		location = time.Local
	}
	return date.In(location).Format("02/01/2006 15:04:05.000")
}

func (fs *FormatService) IsoDate(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.UTC().Format("2006-01-02T15:04:05.000Z")
}

func (fs *FormatService) Day(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	location := fs.Location
	if location == nil {
		location = time.Local
	}
	return date.In(location).Format("2006-01-02")
}

func (fs *FormatService) Filename(t ...time.Time) string {
	return "log-" + fs.Day(t...) + ".txt"
}

func (fs *FormatService) JSONFilename(t ...time.Time) string {
	return "log-" + fs.Day(t...) + ".jsonl"
}

func (fs *FormatService) SegmentFilename(extension string, date time.Time, sequence int) string {
	suffix := ""
	if sequence > 0 {
		suffix = fmt.Sprintf(".%03d", sequence)
	}
	return fmt.Sprintf("log-%s%s.%s", fs.Day(date), suffix, strings.TrimPrefix(extension, "."))
}

// Line formats one event as a single terminal-safe line with short event ID.
func (fs *FormatService) Line(entry types.LogEntry) string {
	timestamp := time.Now()
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		timestamp = parsed
	}
	client := entry.Client
	if client == "" {
		client = fs.Client
	}
	id := fs.ShortID(entry.EventID)
	level := string(entry.Level)
	if len(level) < 5 {
		level += strings.Repeat(" ", 5-len(level))
	}
	parts := make([]string, 0, len(entry.Context)+len(entry.Data)+1)
	parts = appendMetadata(parts, entry.Context)
	parts = appendMetadata(parts, entry.Data)
	if entry.Error != nil {
		details := map[string]any{}
		for _, key := range []string{"name", "message"} {
			if value, ok := entry.Error[key]; ok {
				details[key] = value
			}
		}
		parts = append(parts, "error="+displayValue(details))
	}
	suffix := ""
	if len(parts) > 0 {
		suffix = " > " + strings.Join(parts, " > ")
	}
	message := strings.Join(strings.Fields(safeText(entry.Message)), " ")
	return fmt.Sprintf("[%s] [%s] [%s] [%s] [%s] : %s%s\n",
		headerSafe(client), fs.Date(timestamp), id, level, headerSafe(entry.Code),
		message, suffix,
	)
}

func (fs *FormatService) JSONLine(entry types.LogEntry) string {
	line, err := json.Marshal(entry)
	if err != nil {
		fallback := map[string]string{"eventId": entry.EventID, "client": entry.Client, "level": string(entry.Level), "code": entry.Code, "message": "[JSON serialization failed]"}
		line, _ = json.Marshal(fallback)
	}
	return string(line) + "\n"
}

func appendMetadata(parts []string, values types.LogData) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, safeText(key)+"="+displayValue(values[key]))
	}
	return parts
}

func displayValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.Join(strings.Fields(safeText(text)), " ")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return safeText(fmt.Sprint(value))
	}
	return safeText(string(data))
}

func safeText(text string) string {
	var result strings.Builder
	for _, character := range text {
		switch character {
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			if character < 0x20 || (character >= 0x7f && character <= 0x9f) || character == 0x2028 || character == 0x2029 {
				if character <= 0xffff {
					result.WriteString(`\u`)
					result.WriteString(fmt.Sprintf("%04x", character))
				} else {
					result.WriteString(strconv.QuoteRune(character))
				}
			} else {
				result.WriteRune(character)
			}
		}
	}
	return result.String()
}

func headerSafe(text string) string {
	return strings.NewReplacer("[", `\[`, "]", `\]`).Replace(safeText(text))
}

func Caller(skip int) string {
	for frame := skip; frame < skip+32; frame++ {
		pc, file, line, ok := runtime.Caller(frame)
		if !ok {
			break
		}
		function := runtime.FuncForPC(pc)
		if function != nil {
			name := function.Name()
			if strings.HasPrefix(name, "runtime.") || strings.HasPrefix(name, "testing.") ||
				strings.Contains(name, "github.com/vinialx/vloggo-go/v2/internal.Caller") ||
				strings.Contains(name, "github.com/vinialx/vloggo-go/v2.(*VLoggo).") {
				continue
			}
		}
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}
	return "(unknown:0)"
}
