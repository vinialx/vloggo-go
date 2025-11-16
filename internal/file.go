// Package services provides formatting and file management services for VLoggo.
// Includes FormatService for log formatting and timestamps, and FileService for file operations,
// log rotation and retention management.
package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		cfg:         cfg,
		format:      NewFormatService(cfg.Client),
		currentDay:  0,
		initialized: false,
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

	fs.currentDay = time.Now().Day()

	txtDir := fs.cfg.Directory.Txt
	if err := os.MkdirAll(txtDir, 0755); err != nil {
		return fmt.Errorf("error creating txt directory > %s", err)
	}

	fs.txtFilename = filepath.Join(txtDir, fs.format.Filename())

	if err := fs.appendToFile(fs.txtFilename, fs.format.Separator()); err != nil {
		return fmt.Errorf("error writing txt separator: %w", err)
	}

	if fs.cfg.Json {
		jsonDir := fs.cfg.Directory.Json
		if err := os.MkdirAll(jsonDir, 0755); err != nil {
			return fmt.Errorf("error creating json directory > %s", err)
		}

		fs.jsonFilename = filepath.Join(jsonDir, fs.format.JSONFilename())

		if err := fs.appendToFile(fs.jsonFilename, fs.format.JSONSeparator()); err != nil {
			return fmt.Errorf("error writing json separator: %w", err)
		}
	}

	fs.initialized = true

	fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : FileService initialized\n",
		fs.cfg.Client,
		fs.format.Date(),
	)

	return nil

}

func (fs *FileService) verify() error {
	today := time.Now().Day()

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if today == fs.currentDay {
		return nil
	}

	if fs.cfg.Debug {
		fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : reinitializing vloggo with new file\n",
			fs.cfg.Client,
			fs.format.Date(),
		)
	}

	fs.currentDay = time.Now().Day()

	txtDir := fs.cfg.Directory.Txt
	if err := os.MkdirAll(txtDir, 0755); err != nil {
		return fmt.Errorf("error creating txt directory > %s", err)
	}

	fs.txtFilename = filepath.Join(txtDir, fs.format.Filename())

	if err := fs.appendToFile(fs.txtFilename, fs.format.Separator()); err != nil {
		return fmt.Errorf("error writing txt separator > %w", err)
	}

	if fs.cfg.Json {
		jsonDir := fs.cfg.Directory.Json
		if err := os.MkdirAll(jsonDir, 0755); err != nil {
			return fmt.Errorf("error creating json directory > %s", err)
		}

		fs.jsonFilename = filepath.Join(jsonDir, fs.format.JSONFilename())

		if err := fs.appendToFile(fs.jsonFilename, fs.format.JSONSeparator()); err != nil {
			return fmt.Errorf("error writing json separator > %w", err)
		}
	}

	fs.initialized = true

	if err := fs.rotate(); err != nil {
		return fmt.Errorf("vloggo cleanup failed > %w", err)
	}

	return nil
}

func (fs *FileService) rotate() error {
	// Rotaciona arquivos TXT
	if err := fs.rotateTxt(); err != nil {
		return err
	}

	// Rotaciona arquivos JSON se habilitado
	if fs.cfg.Json {
		if err := fs.rotateJson(); err != nil {
			return err
		}
	}

	return nil
}
func (fs *FileService) rotateTxt() error {

	txtDir := fs.cfg.Directory.Txt

	files, err := os.ReadDir(txtDir)
	if err != nil {
		return fmt.Errorf("error reading txt directory > %w", err)
	}

	type fileInfo struct {
		path  string
		mtime time.Time
	}

	var logFiles []fileInfo
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".txt" {
			filePath := filepath.Join(txtDir, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}
			logFiles = append(logFiles, fileInfo{
				path:  filePath,
				mtime: info.ModTime(),
			})
		}
	}

	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].mtime.After(logFiles[j].mtime)
	})

	if len(logFiles) > fs.cfg.Filecount.Txt {
		filesToDelete := logFiles[fs.cfg.Filecount.Txt:]
		for _, file := range filesToDelete {
			if err := os.Remove(file.path); err != nil {
				fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : error deleting old txt file > %v\n",
					fs.cfg.Client,
					fs.format.Date(),
					err,
				)
			}
		}
	}

	return nil
}

func (fs *FileService) rotateJson() error {
	jsonDir := fs.cfg.Directory.Json

	files, err := os.ReadDir(jsonDir)
	if err != nil {
		return fmt.Errorf("error reading json directory: %w", err)
	}

	type fileInfo struct {
		path  string
		mtime time.Time
	}

	var logFiles []fileInfo
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".jsonl" {
			filePath := filepath.Join(jsonDir, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}
			logFiles = append(logFiles, fileInfo{
				path:  filePath,
				mtime: info.ModTime(),
			})
		}
	}

	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].mtime.After(logFiles[j].mtime)
	})

	if len(logFiles) > fs.cfg.Filecount.Json {
		filesToDelete := logFiles[fs.cfg.Filecount.Json:]
		for _, file := range filesToDelete {
			if err := os.Remove(file.path); err != nil {
				fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : error deleting old json file > %v\n",
					fs.cfg.Client,
					fs.format.Date(),
					err,
				)
			}
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

	if err := fs.verify(); err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : > %v",
			fs.cfg.Client,
			fs.format.Date(),
			err,
		)
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
