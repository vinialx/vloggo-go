package services

import (
	"fmt"
	"os"
	"path/filepath"
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
		if err := os.MkdirAll(txtDir, 0755); err != nil {
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

func (fs *FileService) Write(line string, jsonLine ...string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if !fs.initialized {
		return fmt.Errorf("file service not initialized")
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
