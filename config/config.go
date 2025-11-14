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

func Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

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

func validateSMTP(config types.VLoggoSMTP) error {
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

	err = validateSMTP(smtp)
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

func DefaultConfig() types.VLoggoConfig {
	return types.VLoggoConfig{
		Client:    "VLoggo",
		Json:      false,
		Notify:    true,
		Console:   true,
		Throttle:  30,
		Filecount: types.Count{Txt: 31, Json: 31},
		Directory: DefaultDirectory("VLoggo"),
	}
}

func NewConfig() types.VLoggoConfig {
	return DefaultConfig()
}

func WithClient(cfg types.VLoggoConfig, client string) types.VLoggoConfig {
	cfg.Client = client
	return cfg
}

func WithJSON(cfg types.VLoggoConfig, enabled bool) types.VLoggoConfig {
	cfg.Json = enabled
	return cfg
}

func WithConsole(cfg types.VLoggoConfig, enabled bool) types.VLoggoConfig {
	cfg.Console = enabled
	return cfg
}

func WithThrottle(cfg types.VLoggoConfig, seconds int) types.VLoggoConfig {
	cfg.Throttle = seconds
	return cfg
}

func WithFilecount(cfg types.VLoggoConfig, txt, json int) types.VLoggoConfig {
	cfg.Filecount = types.Count{Txt: txt, Json: json}
	return cfg
}

func WithDirectory(cfg types.VLoggoConfig, txt, json string) types.VLoggoConfig {
	cfg.Directory = types.Paths{Txt: txt, Json: json}
	return cfg
}

func WithSMTP(cfg types.VLoggoConfig, smtp types.VLoggoSMTP) types.VLoggoConfig {
	cfg.SMTP = smtp
	return cfg
}
