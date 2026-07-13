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
	email  *services.EmailService
}

var (
	instances = make(map[string]*VLoggo)
	mu        sync.RWMutex
)

func NewInstance(client string, opts ...config.Option) *VLoggo {
	if client == "" {
		client = config.DefaultClient
	}

	mu.RLock()
	if instance, exists := instances[client]; exists {
		mu.RUnlock()
		return instance
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	cfg := config.DefaultConfig(client)

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	cfg = config.Normalize(cfg)

	if instance, exists := instances[cfg.Client]; exists {
		return instance
	}

	instance := newVLoggo(cfg)

	instances[cfg.Client] = instance

	return instance

}

func NewInstanceFromConfig(cfg types.VLoggoConfig) *VLoggo {
	cfg = config.Normalize(cfg)

	mu.RLock()
	if instance, exists := instances[cfg.Client]; exists {
		mu.RUnlock()
		return instance
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	if instance, exists := instances[cfg.Client]; exists {
		return instance
	}

	instance := newVLoggo(cfg)
	instances[cfg.Client] = instance

	return instance
}

func NewInstanceFromInput(input types.VLoggoConfigInput) *VLoggo {
	return NewInstanceFromConfig(config.FromInput(input))
}

func newVLoggo(cfg types.VLoggoConfig) *VLoggo {
	cfg = config.Normalize(cfg)
	return &VLoggo{
		cfg:    cfg,
		file:   services.NewFileService(cfg),
		format: services.NewFormatService(cfg.Client),
		email:  services.NewEmailService(cfg),
	}
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
	cloneCfg = config.Normalize(cloneCfg)

	mu.Lock()
	defer mu.Unlock()

	if existingInstance, exists := instances[cloneCfg.Client]; exists {
		return existingInstance
	}

	newInstance := newVLoggo(cloneCfg)
	instances[cloneCfg.Client] = newInstance

	fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : instance cloned from %s\n",
		cloneCfg.Client,
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

	v.cfg = config.Normalize(v.cfg)
	v.file = services.NewFileService(v.cfg)
	v.format = services.NewFormatService(v.cfg.Client)
	v.email = services.NewEmailService(v.cfg)
}

func (v *VLoggo) log(level types.LogLevel, code, message string) {
	v.mu.Lock()

	if level == types.Debug && !v.cfg.Debug {
		v.mu.Unlock()
		return
	}

	if v.file == nil || v.format == nil {
		v.mu.Unlock()
		return
	}

	entry := types.LogEntry{
		Level:   level,
		Code:    code,
		Caller:  services.Caller(3),
		Message: message,
	}

	line := v.format.Line(entry)
	if v.cfg.Console {
		fmt.Print(line)
	}

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

	var email *services.EmailService
	client := v.cfg.Client
	if level == types.Fatal && v.cfg.Notify {
		email = v.email
	}
	v.mu.Unlock()

	if email != nil {
		if err := email.NotifyFatal(entry); err != nil {
			fmt.Printf("[VLoggo] > [%s] [%s] [INFO] : failed to send fatal notification > %s\n",
				client,
				config.Date(),
				err,
			)
		}
	}
}

func (v *VLoggo) Info(code, message string) {
	v.log(types.Info, code, message)
}

func (v *VLoggo) Warn(code, message string) {
	v.log(types.Warn, code, message)
}

func (v *VLoggo) Debug(code, message string) {
	v.log(types.Debug, code, message)
}

func (v *VLoggo) Error(code, message string) {
	v.log(types.Error, code, message)
}

func (v *VLoggo) Fatal(code, message string) {
	v.log(types.Fatal, code, message)

	os.Exit(1)
}
