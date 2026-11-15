package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vinialx/vloggo-go/v2/types"
)

type sinkName string

const (
	textSink sinkName = "text"
	jsonSink sinkName = "json"
)

var (
	fileOperationsMu sync.Mutex
	activePathsMu    sync.Mutex
	activePaths      = make(map[string]int)
	errUnsafeLogPath = errors.New("log target is a symlink or changed during open")
)

type fileState struct {
	filename string
	day      string
	sequence int
	ready    bool
}

type managedLog struct {
	filename string
	day      string
	sequence int
	size     int64
	sink     sinkName
}

type FileService struct {
	cfg          types.VLoggoConfig
	format       *FormatService
	files        map[sinkName]fileState
	initialized  bool
	closed       bool
	mu           sync.Mutex
	reportedOnce map[string]bool
}

func NewFileService(cfg types.VLoggoConfig) *FileService {
	return &FileService{
		cfg:          cfg,
		format:       NewFormatService(cfg.Client, cfg.Format.Timezone),
		files:        map[sinkName]fileState{textSink: {}, jsonSink: {}},
		reportedOnce: make(map[string]bool),
	}
}

func (fs *FileService) Initialize() error {
	fileOperationsMu.Lock()
	defer fileOperationsMu.Unlock()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.closed {
		return errors.New("file service is closed")
	}
	return fs.initializeLocked(time.Now())
}

func (fs *FileService) initializeLocked(date time.Time) error {
	var failures []error
	for _, sink := range []sinkName{textSink, jsonSink} {
		if !fs.enabled(sink) {
			fs.setFile(sink, fileState{})
			continue
		}
		if err := fs.ensureLocked(sink, date, 0); err != nil {
			if reportErr := fs.reportOnce(sink, fmt.Errorf("initialize %s log: %w", sink, err)); reportErr != nil {
				failures = append(failures, reportErr)
			}
		}
	}
	fs.initialized = true
	if err := fs.cleanupLocked(date); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (fs *FileService) Write(entry types.LogEntry) error {
	fileOperationsMu.Lock()
	defer fileOperationsMu.Unlock()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.closed {
		return errors.New("file service is closed")
	}
	if !fs.initialized {
		if err := fs.initializeLocked(time.Now()); err != nil && !fs.cfg.Outputs.Text && !fs.cfg.Outputs.JSON {
			return err
		}
	}
	date, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil {
		date = time.Now()
	}
	var failures []error
	for _, sink := range []sinkName{textSink, jsonSink} {
		if !fs.enabled(sink) {
			continue
		}
		content := fs.format.Line(entry)
		if sink == jsonSink {
			content = fs.format.JSONLine(entry)
		}
		if err := fs.ensureLocked(sink, date, int64(len(content))); err != nil {
			if reportErr := fs.reportOnce(sink, fmt.Errorf("select %s log file: %w", sink, err)); reportErr != nil {
				failures = append(failures, reportErr)
			}
			continue
		}
		state := fs.files[sink]
		if err := appendSafe(state.filename, content); err != nil {
			fs.setFile(sink, fileState{day: state.day})
			if errors.Is(err, errUnsafeLogPath) {
				if retryErr := fs.ensureLocked(sink, date, int64(len(content))); retryErr == nil {
					state = fs.files[sink]
					err = appendSafe(state.filename, content)
				} else {
					err = errors.Join(err, retryErr)
				}
			}
			if err != nil {
				fs.setFile(sink, fileState{day: state.day})
				if reportErr := fs.reportOnce(sink, fmt.Errorf("write %s log: %w", sink, err)); reportErr != nil {
					failures = append(failures, reportErr)
				}
				continue
			}
		}
		delete(fs.reportedOnce, string(sink)+":"+fs.directory(sink))
	}
	if err := fs.cleanupLocked(date); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (fs *FileService) Flush() error {
	fileOperationsMu.Lock()
	defer fileOperationsMu.Unlock()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.closed {
		return nil
	}
	return fs.cleanupLocked(time.Now())
}

func (fs *FileService) Close() error {
	fileOperationsMu.Lock()
	defer fileOperationsMu.Unlock()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.closed {
		return nil
	}
	fs.closed = true
	for _, sink := range []sinkName{textSink, jsonSink} {
		fs.setFile(sink, fileState{})
	}
	return nil
}

// reportOnce returns err only for the first failure of a sink directory until it recovers.
func (fs *FileService) reportOnce(sink sinkName, err error) error {
	key := string(sink) + ":" + fs.directory(sink)
	if fs.reportedOnce[key] {
		return nil
	}
	fs.reportedOnce[key] = true
	return err
}

func (fs *FileService) enabled(sink sinkName) bool {
	if sink == textSink {
		return fs.cfg.Outputs.Text
	}
	return fs.cfg.Outputs.JSON
}

func (fs *FileService) directory(sink sinkName) string {
	if sink == textSink {
		return fs.cfg.Directory.Txt
	}
	return fs.cfg.Directory.Json
}

func (fs *FileService) extension(sink sinkName) string {
	if sink == textSink {
		return ".txt"
	}
	return ".jsonl"
}

func (fs *FileService) ensureLocked(sink sinkName, date time.Time, nextBytes int64) error {
	directory := fs.directory(sink)
	day := fs.format.Day(date)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	candidates, err := fs.candidates(sink, day)
	if err != nil {
		return err
	}
	var selected *managedLog
	latest := candidates
	if len(latest) > 0 {
		state := fs.files[sink]
		if state.ready && state.day == day && filepath.Dir(state.filename) == directory && state.sequence == latest[0].sequence {
			if size, statErr := fileSize(state.filename); statErr == nil && (size == 0 || size+nextBytes <= fs.cfg.Rotation.MaxFileSize) {
				selected = &managedLog{filename: state.filename, day: day, sequence: state.sequence, size: size, sink: sink}
			}
		}
		if selected == nil && (latest[0].size == 0 || latest[0].size+nextBytes <= fs.cfg.Rotation.MaxFileSize) {
			selected = &latest[0]
		}
	}
	if selected == nil {
		sequence := 0
		if len(candidates) > 0 {
			sequence = candidates[0].sequence + 1
		}
		for attempt := 0; attempt < 1000; attempt++ {
			filename := filepath.Join(directory, fs.format.SegmentFilename(fs.extension(sink), date, sequence))
			if createErr := appendSafe(filename, ""); createErr == nil {
				selected = &managedLog{filename: filename, day: day, sequence: sequence, sink: sink}
				break
			} else if !errors.Is(createErr, errUnsafeLogPath) {
				return createErr
			}
			sequence++
		}
		if selected == nil {
			return fmt.Errorf("could not allocate a safe log segment in %s", directory)
		}
	} else if err := appendSafe(selected.filename, ""); err != nil {
		return err
	}
	fs.setFile(sink, fileState{filename: selected.filename, day: day, sequence: selected.sequence, ready: true})
	delete(fs.reportedOnce, string(sink)+":"+directory)
	return nil
}

func (fs *FileService) candidates(sink sinkName, day string) ([]managedLog, error) {
	entries, err := os.ReadDir(fs.directory(sink))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]managedLog, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		fileDay, sequence, ok := parseManagedLog(entry.Name(), fs.extension(sink))
		if !ok || fileDay != day {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, managedLog{
			filename: filepath.Join(fs.directory(sink), entry.Name()),
			day:      fileDay, sequence: sequence, size: info.Size(), sink: sink,
		})
	}
	sort.Slice(files, func(left, right int) bool { return files[left].sequence > files[right].sequence })
	return files, nil
}

func (fs *FileService) setFile(sink sinkName, state fileState) {
	previous := fs.files[sink]
	if previous.ready && (!state.ready || previous.filename != state.filename) {
		activePathsMu.Lock()
		activePaths[previous.filename]--
		if activePaths[previous.filename] <= 0 {
			delete(activePaths, previous.filename)
		}
		activePathsMu.Unlock()
	}
	fs.files[sink] = state
	if state.ready && (!previous.ready || previous.filename != state.filename) {
		activePathsMu.Lock()
		activePaths[state.filename]++
		activePathsMu.Unlock()
	}
}

func (fs *FileService) cleanupLocked(now time.Time) error {
	files, err := fs.managedFiles()
	var failures []error
	if err != nil {
		failures = append(failures, err)
	}
	location := fs.format.Location
	today := now.In(location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)
	oldest := today.AddDate(0, 0, -(fs.cfg.Rotation.RetentionDays - 1))
	for _, file := range files {
		fileDate, parseErr := time.ParseInLocation("2006-01-02", file.day, location)
		if parseErr == nil && fileDate.Before(oldest) && !pathIsActive(file.filename) {
			if removeErr := removeManaged(file); removeErr != nil {
				failures = append(failures, removeErr)
			} else {
				files = removeFile(files, file.filename)
			}
		}
	}

	for _, sink := range []sinkName{textSink, jsonSink} {
		if !fs.enabled(sink) {
			continue
		}
		matching := make([]managedLog, 0)
		for _, file := range files {
			if file.sink == sink {
				matching = append(matching, file)
			}
		}
		sort.Slice(matching, func(left, right int) bool {
			if matching[left].day != matching[right].day {
				return matching[left].day > matching[right].day
			}
			return matching[left].sequence > matching[right].sequence
		})
		protected := make(map[string]bool)
		for _, file := range matching {
			if pathIsActive(file.filename) {
				protected[file.filename] = true
			}
		}
		keepNonActive := fs.filecount(sink) - len(protected)
		for _, file := range matching {
			if protected[file.filename] {
				continue
			}
			if keepNonActive > 0 {
				keepNonActive--
				continue
			}
			if removeErr := removeManaged(file); removeErr != nil {
				failures = append(failures, removeErr)
			} else {
				files = removeFile(files, file.filename)
			}
		}
	}

	total := int64(0)
	for _, file := range files {
		total += file.size
	}
	if total > fs.cfg.Rotation.MaxTotalSize {
		sort.Slice(files, func(left, right int) bool {
			if files[left].day != files[right].day {
				return files[left].day < files[right].day
			}
			return files[left].sequence < files[right].sequence
		})
		for _, file := range files {
			if total <= fs.cfg.Rotation.MaxTotalSize {
				break
			}
			if pathIsActive(file.filename) {
				continue
			}
			if removeErr := removeManaged(file); removeErr != nil {
				failures = append(failures, removeErr)
			} else {
				total -= file.size
			}
		}
	}
	return errors.Join(failures...)
}

func (fs *FileService) managedFiles() ([]managedLog, error) {
	filesByPath := make(map[string]managedLog)
	var failures []error
	for _, sink := range []sinkName{textSink, jsonSink} {
		if !fs.enabled(sink) {
			continue
		}
		entries, err := os.ReadDir(fs.directory(sink))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if reportErr := fs.reportOnce(sink, fmt.Errorf("read log directory %s: %w", fs.directory(sink), err)); reportErr != nil {
				failures = append(failures, reportErr)
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			day, sequence, ok := parseManagedLog(entry.Name(), fs.extension(sink))
			if !ok {
				continue
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			filename := filepath.Join(fs.directory(sink), entry.Name())
			filesByPath[filename] = managedLog{filename: filename, day: day, sequence: sequence, size: info.Size(), sink: sink}
		}
	}
	files := make([]managedLog, 0, len(filesByPath))
	for _, file := range filesByPath {
		files = append(files, file)
	}
	return files, errors.Join(failures...)
}

func (fs *FileService) filecount(sink sinkName) int {
	if sink == textSink {
		return fs.cfg.Filecount.Txt
	}
	return fs.cfg.Filecount.Json
}

func parseManagedLog(name, extension string) (string, int, bool) {
	if !strings.HasPrefix(name, "log-") || !strings.HasSuffix(name, extension) {
		return "", 0, false
	}
	stem := strings.TrimSuffix(strings.TrimPrefix(name, "log-"), extension)
	day := stem
	sequence := 0
	if len(stem) > len("2006-01-02") {
		if len(stem) < 12 || stem[10] != '.' {
			return "", 0, false
		}
		day = stem[:10]
		parsed, err := strconv.Atoi(stem[11:])
		if err != nil || parsed < 1 {
			return "", 0, false
		}
		sequence = parsed
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return "", 0, false
	}
	return day, sequence, true
}

func fileSize(filename string) (int64, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errUnsafeLogPath
	}
	return info.Size(), nil
}

func appendSafe(filename, content string) error {
	before, err := os.Lstat(filename)
	if err == nil && !before.Mode().IsRegular() {
		return errUnsafeLogPath
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	after, err := os.Lstat(filename)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return errUnsafeLogPath
	}
	if content == "" {
		return nil
	}
	_, err = file.WriteString(content)
	return err
}

func pathIsActive(filename string) bool {
	activePathsMu.Lock()
	defer activePathsMu.Unlock()
	return activePaths[filename] > 0
}

func removeManaged(file managedLog) error {
	if pathIsActive(file.filename) {
		return nil
	}
	info, err := os.Lstat(file.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat managed log %s: %w", file.filename, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.Remove(file.filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove managed log %s: %w", file.filename, err)
	}
	return nil
}

func removeFile(files []managedLog, filename string) []managedLog {
	result := make([]managedLog, 0, len(files))
	for _, file := range files {
		if file.filename != filename {
			result = append(result, file)
		}
	}
	return result
}
