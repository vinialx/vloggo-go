package vloggo

import (
	"fmt"
	"sync"

	"github.com/vinialx/vloggo-go/config"
	types "github.com/vinialx/vloggo-go/types"
)

type VLoggo struct {
	cfg types.VLoggoConfig
	mu  sync.Mutex
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
		cfg: cfg,
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
