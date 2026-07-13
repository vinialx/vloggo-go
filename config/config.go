// Package config provides defaults and functional options for VLoggo.
package config

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"time"

	types "github.com/vinialx/vloggo-go/types"
)

const (
	DefaultClient       = "VLoggo"
	DefaultThrottleMS   = 30000
	DefaultRetentionDay = 31
)

// Option defines the functional option type used to modify a VLoggoConfig.
type Option func(*types.VLoggoConfig)

// Date returns a formatted date string ("02/01/2006 15:04:05").
func Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

// DefaultDirectory returns default log paths based on the client name.
func DefaultDirectory(client string) types.Paths {
	if client == "" {
		client = DefaultClient
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : home dir not found > %v\n",
			client,
			Date(),
			err,
		)

		return types.Paths{
			Txt:  filepath.Join(".", client, "logs"),
			Json: filepath.Join(".", client, "json"),
		}
	}

	return types.Paths{
		Txt:  filepath.Join(home, client, "logs"),
		Json: filepath.Join(home, client, "json"),
	}
}

// ValidateSMTP checks whether an SMTP configuration can be used.
func ValidateSMTP(config types.VLoggoSMTP) error {
	if config.Host == "" {
		return fmt.Errorf("host is required")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if config.Username == "" {
		return fmt.Errorf("username is required")
	}
	if config.Password == "" {
		return fmt.Errorf("password is required")
	}
	if config.From == "" {
		return fmt.Errorf("from address is required")
	}
	if len(config.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	if _, err := mail.ParseAddress(config.From); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}

	for _, addr := range config.To {
		if _, err := mail.ParseAddress(addr); err != nil {
			return fmt.Errorf("invalid to address %s: %w", addr, err)
		}
	}

	return nil
}

// DefaultConfig creates a VLoggoConfig with sensible defaults and no environment lookup.
func DefaultConfig(client ...string) types.VLoggoConfig {
	name := DefaultClient
	if len(client) > 0 && client[0] != "" {
		name = client[0]
	}

	return types.VLoggoConfig{
		Client:    name,
		Json:      false,
		Notify:    false,
		Debug:     false,
		Console:   true,
		Throttle:  DefaultThrottleMS,
		Filecount: types.Count{Txt: DefaultRetentionDay, Json: DefaultRetentionDay},
		Directory: DefaultDirectory(name),
	}
}

// Normalize fills required zero-value fields on a complete config.
func Normalize(cfg types.VLoggoConfig) types.VLoggoConfig {
	if cfg.Client == "" {
		cfg.Client = DefaultClient
	}

	defaults := DefaultConfig(cfg.Client)
	if cfg.Throttle <= 0 {
		cfg.Throttle = defaults.Throttle
	}
	if cfg.Filecount.Txt <= 0 {
		cfg.Filecount.Txt = defaults.Filecount.Txt
	}
	if cfg.Filecount.Json <= 0 {
		cfg.Filecount.Json = defaults.Filecount.Json
	}
	if cfg.Directory.Txt == "" {
		cfg.Directory.Txt = defaults.Directory.Txt
	}
	if cfg.Directory.Json == "" {
		cfg.Directory.Json = defaults.Directory.Json
	}

	return cfg
}

// FromInput merges a partial remote config into defaults.
func FromInput(input types.VLoggoConfigInput) types.VLoggoConfig {
	client := DefaultClient
	if input.Client != nil && *input.Client != "" {
		client = *input.Client
	}

	cfg := DefaultConfig(client)
	ApplyInput(&cfg, input)
	return Normalize(cfg)
}

// ApplyInput applies a partial remote config to an existing config.
func ApplyInput(cfg *types.VLoggoConfig, input types.VLoggoConfigInput) {
	if cfg == nil {
		return
	}

	if input.Client != nil && *input.Client != "" {
		cfg.Client = *input.Client
	}
	if input.Json != nil {
		cfg.Json = *input.Json
	}
	if input.Notify != nil {
		cfg.Notify = *input.Notify
	}
	if input.Debug != nil {
		cfg.Debug = *input.Debug
	}
	if input.Console != nil {
		cfg.Console = *input.Console
	}
	if input.Throttle != nil {
		cfg.Throttle = *input.Throttle
	}
	if input.Filecount != nil {
		if input.Filecount.Txt != nil {
			cfg.Filecount.Txt = *input.Filecount.Txt
		}
		if input.Filecount.Json != nil {
			cfg.Filecount.Json = *input.Filecount.Json
		}
	}
	if input.Directory != nil {
		if input.Directory.Txt != nil {
			cfg.Directory.Txt = *input.Directory.Txt
		}
		if input.Directory.Json != nil {
			cfg.Directory.Json = *input.Directory.Json
		}
	}
	if input.SMTP != nil {
		if input.SMTP.Host != nil {
			cfg.SMTP.Host = *input.SMTP.Host
		}
		if input.SMTP.Port != nil {
			cfg.SMTP.Port = *input.SMTP.Port
		}
		if input.SMTP.Username != nil {
			cfg.SMTP.Username = *input.SMTP.Username
		}
		if input.SMTP.Password != nil {
			cfg.SMTP.Password = *input.SMTP.Password
		}
		if input.SMTP.From != nil {
			cfg.SMTP.From = *input.SMTP.From
		}
		if input.SMTP.To != nil {
			cfg.SMTP.To = input.SMTP.To
		}
		if input.SMTP.Secure != nil {
			cfg.SMTP.Secure = *input.SMTP.Secure
		}
	}
}

func WithClient(client string) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Client = client
	}
}

func WithJSON(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Json = enabled
	}
}

func WithDebug(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Debug = enabled
	}
}

func WithConsole(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Console = enabled
	}
}

func WithNotify(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Notify = enabled
	}
}

func WithThrottle(milliseconds int) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Throttle = milliseconds
	}
}

func WithFilecount(filecount types.Count) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Filecount = filecount
	}
}

func WithDirectory(paths types.Paths) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Directory = paths
	}
}

func WithSMTP(smtp types.VLoggoSMTP) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.SMTP = smtp
	}
}

func WithInput(input types.VLoggoConfigInput) Option {
	return func(cfg *types.VLoggoConfig) {
		ApplyInput(cfg, input)
	}
}
