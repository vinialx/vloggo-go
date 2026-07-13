package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	types "github.com/vinialx/vloggo-go/types"
)

type FileService struct {
	cfg          types.VLoggoConfig
	txtFilename  string
	jsonFilename string

	currentDay  int
	format      *FormatService
	initialized bool
	mu          sync.Mutex
}

func NewFileService(cfg types.VLoggoConfig) *FileService {
	fs := &FileService{
		cfg:    cfg,
		format: NewFormatService(cfg.Client),
	}

	if err := fs.Initialize(); err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : failed to initialize FileService > %v\n",
			cfg.Client,
			fs.format.Date(),
			err,
		)
	}

	return fs
}

func (fs *FileService) appendToFile(filename, content string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}

func (fs *FileService) Initialize() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.initialized {
		return nil
	}

	return fs.initializeLocked()
}

func (fs *FileService) initializeLocked() error {
	fs.currentDay = time.Now().Day()

	if err := os.MkdirAll(fs.cfg.Directory.Txt, 0755); err != nil {
		return fmt.Errorf("error creating txt directory > %w", err)
	}

	fs.txtFilename = filepath.Join(fs.cfg.Directory.Txt, fs.format.Filename())
	if err := fs.appendToFile(fs.txtFilename, fs.format.Separator()); err != nil {
		return fmt.Errorf("error writing txt separator > %w", err)
	}

	if fs.cfg.Json {
		if err := os.MkdirAll(fs.cfg.Directory.Json, 0755); err != nil {
			return fmt.Errorf("error creating json directory > %w", err)
		}

		fs.jsonFilename = filepath.Join(fs.cfg.Directory.Json, fs.format.JSONFilename())
		if err := fs.appendToFile(fs.jsonFilename, fs.format.JSONSeparator()); err != nil {
			return fmt.Errorf("error writing json separator > %w", err)
		}
	}

	fs.initialized = true
	if err := fs.rotateLocked(); err != nil {
		return err
	}

	fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : FileService initialized\n",
		fs.cfg.Client,
		fs.format.Date(),
	)

	return nil
}

func (fs *FileService) verifyLocked() error {
	today := time.Now().Day()
	if today == fs.currentDay {
		return nil
	}

	if fs.cfg.Debug {
		fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : reinitializing vloggo with new file\n",
			fs.cfg.Client,
			fs.format.Date(),
		)
	}

	fs.currentDay = today

	if err := os.MkdirAll(fs.cfg.Directory.Txt, 0755); err != nil {
		return fmt.Errorf("error creating txt directory > %w", err)
	}

	fs.txtFilename = filepath.Join(fs.cfg.Directory.Txt, fs.format.Filename())
	if err := fs.appendToFile(fs.txtFilename, fs.format.Separator()); err != nil {
		return fmt.Errorf("error writing txt separator > %w", err)
	}

	if fs.cfg.Json {
		if err := os.MkdirAll(fs.cfg.Directory.Json, 0755); err != nil {
			return fmt.Errorf("error creating json directory > %w", err)
		}

		fs.jsonFilename = filepath.Join(fs.cfg.Directory.Json, fs.format.JSONFilename())
		if err := fs.appendToFile(fs.jsonFilename, fs.format.JSONSeparator()); err != nil {
			return fmt.Errorf("error writing json separator > %w", err)
		}
	}

	return fs.rotateLocked()
}

func (fs *FileService) rotateLocked() error {
	if err := fs.rotateDir(fs.cfg.Directory.Txt, ".txt", fs.cfg.Filecount.Txt); err != nil {
		return err
	}

	if fs.cfg.Json {
		if err := fs.rotateDir(fs.cfg.Directory.Json, ".jsonl", fs.cfg.Filecount.Json); err != nil {
			return err
		}
	}

	return nil
}

func (fs *FileService) rotateDir(dir, ext string, keep int) error {
	if keep <= 0 {
		return nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("error reading %s directory > %w", ext, err)
	}

	type logFile struct {
		path string
		date time.Time
	}

	logFiles := make([]logFile, 0, len(files))
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ext {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		fileDate := info.ModTime()
		datePart := strings.TrimSuffix(strings.TrimPrefix(file.Name(), "log-"), ext)
		if parsed, err := time.Parse("2006-01-02", datePart); err == nil {
			fileDate = parsed
		}

		logFiles = append(logFiles, logFile{
			path: filepath.Join(dir, file.Name()),
			date: fileDate,
		})
	}

	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].date.After(logFiles[j].date)
	})

	if len(logFiles) <= keep {
		return nil
	}

	for _, file := range logFiles[keep:] {
		if err := os.Remove(file.path); err != nil {
			fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : error deleting old log file > %v\n",
				fs.cfg.Client,
				fs.format.Date(),
				err,
			)
		}
	}

	return nil
}

func (fs *FileService) Write(line string, jsonLine ...string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if !fs.initialized {
		return fmt.Errorf("file service not initialized")
	}

	if err := fs.verifyLocked(); err != nil {
		return err
	}

	if err := fs.appendToFile(fs.txtFilename, line); err != nil {
		return fmt.Errorf("error writing txt > %w", err)
	}

	if fs.cfg.Json && len(jsonLine) > 0 {
		if err := fs.appendToFile(fs.jsonFilename, jsonLine[0]); err != nil {
			return fmt.Errorf("error writing json > %w", err)
		}
	}

	return nil
}
