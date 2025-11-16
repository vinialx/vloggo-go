// Package config provides helper functions and functional options
// for creating and customizing the VLoggoConfig struct.
// It handles default settings (like directories and SMTP),
// environment variable loading, and configuration validation.
package config

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	types "github.com/vinialx/vloggo-go/types"

	"github.com/joho/godotenv"
)

// Option defines the functional option type used to modify a VLoggoConfig struct.
type Option func(*types.VLoggoConfig)

// Date returns a formatted date string ("02/01/2006 15:04:05").
// If no time.Time (t) is provided, it defaults to time.Now().
func Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

// DefaultDirectory returns a types.Paths struct containing default paths
// for Txt and Json logs, based on the client name.
// It attempts to use the user's home directory, falling back to a "C:\" path
// if the home directory cannot be found.
func DefaultDirectory(client string) types.Paths {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : home dir not found > %v",
			client,
			Date(),
			err,
		)

		txtDir := filepath.Join("C:\\", client, "logs")
		jsonDir := filepath.Join("C:\\", client, "json")
		return types.Paths{Txt: txtDir, Json: jsonDir}
	}

	txtDir := filepath.Join(home, client, "logs")
	jsonDir := filepath.Join(home, client, "json")

	return types.Paths{Txt: txtDir, Json: jsonDir}
}

// ValidateSMTP checks if a VLoggoSMTP configuration is valid.
// It verifies required fields (Host, Port, Username, Password, From, To)
// and checks the format of email addresses.
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

// DefaultSMTP attempts to load SMTP configuration from environment variables
// (loading a .env file if present).
// It returns a boolean indicating if the loaded configuration is valid (notify)
// and the VLoggoSMTP struct itself.
func DefaultSMTP(client string) (bool, types.VLoggoSMTP) {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : error loading env config > %v",
			client,
			Date(),
			err,
		)

		return false, types.VLoggoSMTP{
			Host:     "",
			Port:     0,
			Username: "",
			Password: "",
			From:     "",
			To:       []string{""},
		}
	}

	port, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : error converting port to int > %v",
			client,
			Date(),
			err,
		)

		return false, types.VLoggoSMTP{
			Host:     "",
			Port:     0,
			Username: "",
			Password: "",
			From:     "",
			To:       []string{""},
		}
	}

	smtp := types.VLoggoSMTP{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     port,
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
		To:       strings.Split(os.Getenv("SMTP_TO"), ","),
	}

	err = ValidateSMTP(smtp)
	if err != nil {
		fmt.Printf("[VLoggo] > [%s] [%s] [ERROR] : error validating smtp config > %v",
			client,
			Date(),
			err,
		)

		return false, types.VLoggoSMTP{
			Host:     "",
			Port:     0,
			Username: "",
			Password: "",
			From:     "",
			To:       []string{""},
		}
	}

	return true, smtp
}

// DefaultConfig creates and returns a VLoggoConfig struct populated with
// default values. It calls DefaultSMTP and DefaultDirectory to set
// the default SMTP and path settings.
func DefaultConfig() types.VLoggoConfig {
	notify, smtp := DefaultSMTP("VLoggo")

	return types.VLoggoConfig{
		Client:    "VLoggo",
		Json:      false,
		Notify:    notify,
		Debug:     true,
		Console:   true,
		Throttle:  30,
		Filecount: types.Count{Txt: 31, Json: 31},
		Directory: DefaultDirectory("VLoggo"),
		SMTP:      smtp,
	}
}

// WithClient returns an Option function that sets the Client field
// of a VLoggoConfig.
func WithClient(cfg types.VLoggoConfig, client string) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Client = client
	}
}

// WithJSON returns an Option function that sets the Json (enabled) field
// of a VLoggoConfig.
func WithJSON(cfg types.VLoggoConfig, enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Json = enabled
	}
}

// WithDebug returns an Option function that sets the Debug (enabled) field
// of a VLoggoConfig.
func WithDebug(cfg types.VLoggoConfig, enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Debug = enabled
	}
}

// WithConsole returns an Option function that sets the Console (enabled) field
// of a VLoggoConfig.
func WithConsole(cfg types.VLoggoConfig, enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Console = enabled
	}
}

// WithThrottle returns an Option function that sets the Throttle (seconds) field
// of a VLoggoConfig.
func WithThrottle(cfg types.VLoggoConfig, seconds int) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Throttle = seconds
	}
}

// WithFilecount returns an Option function that sets the Filecount (Txt/Json) field
// of a VLoggoConfig.
func WithFilecount(cfg types.VLoggoConfig, filecount types.Count) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Filecount = filecount
	}
}

// WithDirectory returns an Option function that sets the Directory (paths) field
// of a VLoggoConfig.
func WithDirectory(cfg types.VLoggoConfig, paths types.Paths) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Directory = paths
	}
}

// WithSMTP returns an Option function that sets the SMTP field
// of a VLoggoConfig.
func WithSMTP(cfg types.VLoggoConfig, smtp types.VLoggoSMTP) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.SMTP = smtp
	}
}
