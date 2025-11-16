package vloggo

import (
	"fmt"
	"os"
	"sync"

	config "github.com/vinialx/vloggo-go/config"
	services "github.com/vinialx/vloggo-go/internal"
	types "github.com/vinialx/vloggo-go/types"
)

type VLoggo struct {
	mu sync.Mutex

	cfg    types.VLoggoConfig
	file   *services.FileService
	format *services.FormatService
}

var (
	instances = make(map[string]*VLoggo)
	mu        sync.RWMutex
)

func NewInstance(client string, opts ...config.Option) *VLoggo {
	mu.RLock()
	if instance, exists := instances[client]; exists {
		return instance
	}

	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	if instance, exists := instances[client]; exists {
		return instance
	}

	cfg := config.DefaultConfig()
	cfg.Client = client

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	instance := &VLoggo{
		cfg:    cfg,
		file:   services.NewFileService(cfg),
		format: services.NewFormatService(cfg.Client),
	}

	instances[client] = instance

	return instance

}

func GetAllInstances() map[string]*VLoggo {
	mu.RLock()
	defer mu.RUnlock()

	result := make(map[string]*VLoggo, len(instances))
	for k, v := range instances {
		result[k] = v
	}

	return result
}

func (v *VLoggo) GetConfig() types.VLoggoConfig {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.cfg
}

func RemoveInstance(client string) bool {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := instances[client]; exists {
		delete(instances, client)
		fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : instance removed\n",
			client,
			config.Date(),
		)
		return true
	}
	return false
}

func ClearInstances() {
	mu.Lock()
	defer mu.Unlock()

	instances = make(map[string]*VLoggo)
	fmt.Printf("[VLoggo] > [%s] [INFO] : all instances removed \n",
		config.Date(),
	)
}

func Clone(base, client string, opts ...config.Option) *VLoggo {
	mu.RLock()
	baseInstance, exists := instances[base]
	mu.RUnlock()

	if !exists {
		fmt.Printf("[VLoggo] > [%s] [INFO] : instance %s not found\n",
			config.Date(),
			base,
		)
		return nil
	}

	mu.RLock()
	if existingInstance, exists := instances[client]; exists {
		mu.RUnlock()
		fmt.Printf("[VLoggo] > [%s] [INFO] : instance %s already exists\n",
			config.Date(),
			client,
		)
		return existingInstance
	}
	mu.RUnlock()

	cloneCfg := baseInstance.GetConfig()
	cloneCfg.Client = client

	for _, opt := range opts {
		if opt != nil {
			opt(&cloneCfg)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if existingInstance, exists := instances[client]; exists {
		return existingInstance
	}

	newInstance := &VLoggo{
		cfg: cloneCfg,
	}
	instances[client] = newInstance

	fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : instance cloned from %s\n",
		client,
		config.Date(),
		base,
	)

	return newInstance
}

func (v *VLoggo) Update(opts ...config.Option) {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, opt := range opts {
		if opt != nil {
			opt(&v.cfg)
		}
	}
}

func (v *VLoggo) log(level types.LogLevel, code, message string) {

	entry := types.LogEntry{
		Level:   level,
		Code:    code,
		Caller:  services.Caller(3),
		Message: message,
	}

	line := v.format.Line(entry)

	if v.cfg.Json {
		jsonLine := v.format.JSONLine(entry)

		if err := v.file.Write(line, jsonLine); err != nil {
			fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : failed to write to log file > %s\n",
				v.cfg.Client,
				v.format.Date(),
				err,
			)
		}
	} else {
		if err := v.file.Write(line); err != nil {
			fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : failed to write to log file > %s\n",
				v.cfg.Client,
				v.format.Date(),
				err,
			)
		}
	}

}

func (v *VLoggo) Info(code, message string) {
	v.log("INFO", code, message)
}

func (v *VLoggo) Warn(code, message string) {
	v.log("WARN", code, message)
}

func (v *VLoggo) Debug(code, message string) {
	v.log("DEBUG", code, message)
}

func (v *VLoggo) Error(code, message string) {
	v.log("ERROR", code, message)
}

func (v *VLoggo) Fatal(code, message string) {
	v.log("FATAL", code, message)

	os.Exit(1)
}
