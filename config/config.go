// Package config provides defaults, validation, environment resolution, and functional options.
package config

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/vinialx/vloggo-go/v2/types"
)

const (
	DefaultClient                      = "VLoggo"
	DefaultThrottleMS                  = 30000
	DefaultNotificationTimeoutMS       = 10000
	DefaultRetryDelayMS                = 1000
	DefaultRetentionDay                = 31
	DefaultMaxFileSize           int64 = 20 * 1024 * 1024
	DefaultMaxTotalSize          int64 = 1024 * 1024 * 1024
)

type Option func(*types.VLoggoConfig)

func Date(t ...time.Time) string {
	date := time.Now()
	if len(t) > 0 {
		date = t[0]
	}
	return date.Format("02/01/2006 15:04:05")
}

func DefaultDirectory(client string) types.Paths {
	if client == "" {
		client = defaultClient()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return types.Paths{Txt: filepath.Join(".", client, "logs"), Json: filepath.Join(".", client, "json")}
	}
	return types.Paths{Txt: filepath.Join(home, client, "logs"), Json: filepath.Join(home, client, "json")}
}

func DefaultConfig(client ...string) types.VLoggoConfig {
	name := defaultClient()
	if len(client) > 0 && client[0] != "" {
		name = client[0]
	}
	return types.VLoggoConfig{
		Client:     name,
		Console:    true,
		Outputs:    types.VLoggoOutputs{Console: true, Text: true},
		OutputsSet: true,
		Throttle:   DefaultThrottleMS,
		Filecount:  types.Count{Txt: DefaultRetentionDay, Json: DefaultRetentionDay},
		Directory:  DefaultDirectory(name),
		Format:     types.VLoggoFormat{Timezone: time.Local.String()},
		Rotation: types.VLoggoRotation{
			RetentionDays: DefaultRetentionDay,
			MaxFileSize:   DefaultMaxFileSize,
			MaxTotalSize:  DefaultMaxTotalSize,
		},
		NotificationTimeout: DefaultNotificationTimeoutMS,
		RetryDelay:          DefaultRetryDelayMS,
	}
}

// Normalize fills omitted values while preserving explicit zero values tracked by option/input flags.
func Normalize(cfg types.VLoggoConfig) types.VLoggoConfig {
	if cfg.Client == "" && !cfg.ClientSet {
		cfg.Client = defaultClient()
	}
	cfg.NotificationIssue = ""
	defaults := DefaultConfig(cfg.Client)
	if !cfg.OutputsSet && cfg.Outputs != (types.VLoggoOutputs{}) {
		cfg.OutputsSet = true
	}
	if !cfg.OutputsSet && !cfg.ConsoleSet && !cfg.Console {
		cfg.Console = true
	}
	if !cfg.OutputsSet {
		cfg.Outputs = types.VLoggoOutputs{Console: cfg.Console, Text: true, JSON: cfg.Json}
	} else {
		cfg.Console = cfg.Outputs.Console
		cfg.Json = cfg.Outputs.JSON
	}
	if cfg.Level == "" && !cfg.LevelSet {
		cfg.Level = types.Info
		if cfg.Debug {
			cfg.Level = types.Debug
		}
	} else if cfg.Level != "" {
		cfg.Level = types.LogLevel(strings.ToUpper(string(cfg.Level)))
	}
	cfg.Debug = cfg.Level == types.Debug
	if cfg.Throttle == 0 && !cfg.ThrottleSet {
		cfg.Throttle = defaults.Throttle
	}
	if cfg.Filecount.Txt == 0 && !cfg.FilecountTxtSet {
		cfg.Filecount.Txt = defaults.Filecount.Txt
	}
	if cfg.Filecount.Json == 0 && !cfg.FilecountJsonSet {
		cfg.Filecount.Json = defaults.Filecount.Json
	}
	if cfg.Directory.Txt == "" && !cfg.DirectoryTxtSet {
		cfg.Directory.Txt = defaults.Directory.Txt
	}
	if cfg.Directory.Json == "" && !cfg.DirectoryJsonSet {
		cfg.Directory.Json = defaults.Directory.Json
	}
	if cfg.Directory.Txt != "" {
		if absolute, err := filepath.Abs(cfg.Directory.Txt); err == nil {
			cfg.Directory.Txt = absolute
		}
	}
	if cfg.Directory.Json != "" {
		if absolute, err := filepath.Abs(cfg.Directory.Json); err == nil {
			cfg.Directory.Json = absolute
		}
	}
	if cfg.Format.Timezone == "" && !cfg.FormatTimezoneSet {
		cfg.Format.Timezone = defaults.Format.Timezone
	}
	if cfg.Rotation.RetentionDays == 0 && !cfg.RetentionDaysSet {
		cfg.Rotation.RetentionDays = defaults.Rotation.RetentionDays
	}
	if cfg.Rotation.MaxFileSize == 0 && !cfg.MaxFileSizeSet {
		cfg.Rotation.MaxFileSize = defaults.Rotation.MaxFileSize
	}
	if cfg.Rotation.MaxTotalSize == 0 && !cfg.MaxTotalSizeSet {
		cfg.Rotation.MaxTotalSize = defaults.Rotation.MaxTotalSize
	}
	if cfg.NotificationTimeout == 0 && !cfg.NotificationTimeoutSet {
		cfg.NotificationTimeout = defaults.NotificationTimeout
	}
	if cfg.RetryDelay == 0 && !cfg.RetryDelaySet {
		cfg.RetryDelay = defaults.RetryDelay
	}
	if !cfg.SMTP.SecureSet {
		if cfg.SMTP.Secure && cfg.SMTP.Port != 465 {
			// Secure cannot have been inferred on another port, so it was set explicitly.
			cfg.SMTP.SecureSet = true
		} else {
			cfg.SMTP.Secure = cfg.SMTP.Port == 465
		}
	}
	if cfg.SMTP.ConnectionTimeout == 0 && !cfg.SMTP.ConnectionTimeoutSet {
		cfg.SMTP.ConnectionTimeout = defaults.NotificationTimeout
	}
	if cfg.SMTP.GreetingTimeout == 0 && !cfg.SMTP.GreetingTimeoutSet {
		cfg.SMTP.GreetingTimeout = defaults.NotificationTimeout
	}
	if cfg.SMTP.SocketTimeout == 0 && !cfg.SMTP.SocketTimeoutSet {
		cfg.SMTP.SocketTimeout = defaults.NotificationTimeout
	}
	return cfg
}

// Resolve applies environment defaults and infers notifications only from complete SMTP settings.
func Resolve(cfg types.VLoggoConfig) (types.VLoggoConfig, error) {
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return cfg, err
	}
	if !cfg.SMTPSet && smtpEmpty(cfg.SMTP) && (!cfg.NotifySet || cfg.Notify) {
		cfg.SMTP, cfg.SMTPSet = smtpFromEnv(), smtpEnvironmentPresent()
	}
	if !cfg.NotifySet && !cfg.Notify && !smtpEmpty(cfg.SMTP) && ValidateSMTP(cfg.SMTP) == nil {
		cfg.Notify = true
	}
	if cfg.Notify {
		if err := ValidateSMTP(cfg.SMTP); err != nil {
			cfg.NotificationIssue = err.Error()
		}
	}
	return cfg, nil
}

func Validate(cfg types.VLoggoConfig) error {
	if strings.TrimSpace(cfg.Client) == "" || hasControl(cfg.Client) {
		return fmt.Errorf("client must be a nonempty string without control characters")
	}
	unsafeClient := strings.ContainsAny(cfg.Client, "/\\") || cfg.Client == "." || cfg.Client == ".."
	defaultPaths := DefaultDirectory(cfg.Client)
	if unsafeClient && (cfg.Directory.Txt == defaultPaths.Txt || cfg.Directory.Json == defaultPaths.Json) {
		return fmt.Errorf("client must be a single directory name when using default directories")
	}
	if !validLevel(cfg.Level) {
		return fmt.Errorf("level must be one of DEBUG, INFO, WARN, ERROR, FATAL")
	}
	if cfg.Throttle < 0 {
		return fmt.Errorf("throttle must be nonnegative")
	}
	if cfg.NotificationTimeout < 1 {
		return fmt.Errorf("notification timeout must be positive")
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("retry delay must be nonnegative")
	}
	if cfg.Filecount.Txt < 1 || cfg.Filecount.Json < 1 {
		return fmt.Errorf("filecount values must be positive")
	}
	if cfg.Rotation.RetentionDays < 1 || cfg.Rotation.MaxFileSize < 1 || cfg.Rotation.MaxTotalSize < 1 {
		return fmt.Errorf("rotation retention and size limits must be positive")
	}
	if strings.TrimSpace(cfg.Format.Timezone) == "" || hasControl(cfg.Format.Timezone) {
		return fmt.Errorf("format timezone must be a nonempty IANA timezone")
	}
	if _, err := time.LoadLocation(cfg.Format.Timezone); err != nil {
		return fmt.Errorf("invalid IANA timezone %q: %w", cfg.Format.Timezone, err)
	}
	for _, directory := range []string{cfg.Directory.Txt, cfg.Directory.Json} {
		if strings.TrimSpace(directory) == "" || hasControl(directory) {
			return fmt.Errorf("log directory must be a nonempty path without control characters")
		}
	}
	for _, field := range cfg.Redact {
		if strings.TrimSpace(field) == "" || hasControl(field) {
			return fmt.Errorf("redaction paths must be nonempty strings without control characters")
		}
	}
	for name, value := range map[string]int{
		"SMTP connection timeout": cfg.SMTP.ConnectionTimeout,
		"SMTP greeting timeout":   cfg.SMTP.GreetingTimeout,
		"SMTP socket timeout":     cfg.SMTP.SocketTimeout,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be nonnegative", name)
		}
	}
	if (cfg.SMTP.ConnectionTimeoutSet && cfg.SMTP.ConnectionTimeout == 0) ||
		(cfg.SMTP.GreetingTimeoutSet && cfg.SMTP.GreetingTimeout == 0) ||
		(cfg.SMTP.SocketTimeoutSet && cfg.SMTP.SocketTimeout == 0) {
		return fmt.Errorf("SMTP timeouts must be positive")
	}
	return nil
}

func ValidateSMTP(smtp types.VLoggoSMTP) error {
	if strings.TrimSpace(smtp.Host) == "" || hasControl(smtp.Host) {
		return fmt.Errorf("host is required")
	}
	if smtp.Port < 1 || smtp.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	for name, value := range map[string]string{"username": smtp.Username, "password": smtp.Password} {
		if strings.TrimSpace(value) == "" || hasControl(value) {
			return fmt.Errorf("%s is required", name)
		}
	}
	if strings.TrimSpace(smtp.From) == "" || hasControl(smtp.From) {
		return fmt.Errorf("from address is required")
	}
	if len(smtp.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	if _, err := mail.ParseAddress(smtp.From); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	for _, address := range smtp.To {
		if hasControl(address) {
			return fmt.Errorf("invalid recipient address")
		}
		if _, err := mail.ParseAddress(address); err != nil {
			return fmt.Errorf("invalid recipient address: %w", err)
		}
	}
	return nil
}

func FromInput(input types.VLoggoConfigInput) types.VLoggoConfig {
	cfg := DefaultConfig()
	if input.Client != nil && *input.Client != "" {
		cfg = DefaultConfig(*input.Client)
	}
	ApplyInput(&cfg, input)
	return Normalize(cfg)
}

func ApplyInput(cfg *types.VLoggoConfig, input types.VLoggoConfigInput) {
	if cfg == nil {
		return
	}
	*cfg = Normalize(*cfg)
	if input.Client != nil {
		if *input.Client != "" && *input.Client != cfg.Client {
			previous := DefaultDirectory(cfg.Client)
			next := DefaultDirectory(*input.Client)
			if !cfg.DirectoryTxtSet && cfg.Directory.Txt == previous.Txt {
				cfg.Directory.Txt = next.Txt
			}
			if !cfg.DirectoryJsonSet && cfg.Directory.Json == previous.Json {
				cfg.Directory.Json = next.Json
			}
		}
		cfg.Client, cfg.ClientSet = *input.Client, true
	}
	if input.Json != nil {
		cfg.Json = *input.Json
		if cfg.OutputsSet {
			cfg.Outputs.JSON = *input.Json
		}
	}
	if input.Notify != nil {
		cfg.Notify, cfg.NotifySet = *input.Notify, true
	}
	if input.Debug != nil {
		cfg.Debug = *input.Debug
		if !cfg.LevelSet {
			cfg.Level = ""
		}
	}
	if input.Console != nil {
		cfg.Console, cfg.ConsoleSet = *input.Console, true
		if cfg.OutputsSet {
			cfg.Outputs.Console = *input.Console
		}
	}
	if input.Throttle != nil {
		cfg.Throttle, cfg.ThrottleSet = *input.Throttle, true
	}
	if input.Filecount != nil {
		if input.Filecount.Txt != nil {
			cfg.Filecount.Txt, cfg.FilecountTxtSet = *input.Filecount.Txt, true
		}
		if input.Filecount.Json != nil {
			cfg.Filecount.Json, cfg.FilecountJsonSet = *input.Filecount.Json, true
		}
	}
	if input.Directory != nil {
		if input.Directory.Txt != nil {
			cfg.Directory.Txt, cfg.DirectoryTxtSet = *input.Directory.Txt, true
		}
		if input.Directory.Json != nil {
			cfg.Directory.Json, cfg.DirectoryJsonSet = *input.Directory.Json, true
		}
	}
	if input.SMTP != nil {
		cfg.SMTPSet = true
		applySMTPInput(&cfg.SMTP, *input.SMTP)
	}
	if input.Outputs != nil {
		outputs := cfg.Outputs
		if !cfg.OutputsSet {
			outputs = types.VLoggoOutputs{Console: cfg.Console, Text: true, JSON: cfg.Json}
		}
		if input.Outputs.Console != nil {
			outputs.Console = *input.Outputs.Console
		}
		if input.Outputs.Text != nil {
			outputs.Text = *input.Outputs.Text
		}
		if input.Outputs.JSON != nil {
			outputs.JSON = *input.Outputs.JSON
		}
		cfg.Outputs, cfg.OutputsSet = outputs, true
		cfg.Console, cfg.Json = outputs.Console, outputs.JSON
	}
	if input.Level != nil {
		cfg.Level, cfg.LevelSet = *input.Level, true
	}
	if input.Format != nil && input.Format.Timezone != nil {
		cfg.Format.Timezone, cfg.FormatTimezoneSet = *input.Format.Timezone, true
	}
	if input.Rotation != nil {
		if input.Rotation.RetentionDays != nil {
			cfg.Rotation.RetentionDays, cfg.RetentionDaysSet = *input.Rotation.RetentionDays, true
		}
		if input.Rotation.MaxFileSize != nil {
			cfg.Rotation.MaxFileSize, cfg.MaxFileSizeSet = *input.Rotation.MaxFileSize, true
		}
		if input.Rotation.MaxTotalSize != nil {
			cfg.Rotation.MaxTotalSize, cfg.MaxTotalSizeSet = *input.Rotation.MaxTotalSize, true
		}
	}
	if input.Redact != nil {
		cfg.Redact = append([]string(nil), (*input.Redact)...)
	}
	if input.StartupMessage != nil {
		cfg.StartupMessage = *input.StartupMessage
	}
	if input.NotificationTimeout != nil {
		cfg.NotificationTimeout, cfg.NotificationTimeoutSet = *input.NotificationTimeout, true
	}
	if input.RetryDelay != nil {
		cfg.RetryDelay, cfg.RetryDelaySet = *input.RetryDelay, true
	}
}

func WithClient(client string) Option {
	return func(cfg *types.VLoggoConfig) { cfg.Client, cfg.ClientSet = client, true }
}

func WithJSON(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Json = enabled
		if cfg.OutputsSet {
			cfg.Outputs.JSON = enabled
		}
	}
}

func WithDebug(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Debug = enabled
		if !cfg.LevelSet {
			cfg.Level = ""
		}
	}
}

func WithLevel(level types.LogLevel) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Level, cfg.LevelSet = level, true
	}
}

func WithConsole(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Console, cfg.ConsoleSet = enabled, true
		if cfg.OutputsSet {
			cfg.Outputs.Console = enabled
		}
	}
}

func WithOutputs(outputs types.VLoggoOutputs) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Outputs, cfg.OutputsSet = outputs, true
		cfg.Console, cfg.Json, cfg.ConsoleSet = outputs.Console, outputs.JSON, true
	}
}

func WithNotify(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) { cfg.Notify, cfg.NotifySet = enabled, true }
}

func WithThrottle(milliseconds int) Option {
	return func(cfg *types.VLoggoConfig) { cfg.Throttle, cfg.ThrottleSet = milliseconds, true }
}

func WithRetryDelay(milliseconds int) Option {
	return func(cfg *types.VLoggoConfig) { cfg.RetryDelay, cfg.RetryDelaySet = milliseconds, true }
}

func WithNotificationTimeout(milliseconds int) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.NotificationTimeout, cfg.NotificationTimeoutSet = milliseconds, true
	}
}

func WithStartupMessage(enabled bool) Option {
	return func(cfg *types.VLoggoConfig) { cfg.StartupMessage = enabled }
}

func WithRedact(fields ...string) Option {
	return func(cfg *types.VLoggoConfig) { cfg.Redact = append([]string(nil), fields...) }
}

func WithFormat(format types.VLoggoFormat) Option {
	return func(cfg *types.VLoggoConfig) { cfg.Format = format }
}

func WithRotation(rotation types.VLoggoRotation) Option {
	return func(cfg *types.VLoggoConfig) {
		if rotation.RetentionDays != 0 {
			cfg.Rotation.RetentionDays = rotation.RetentionDays
		}
		if rotation.MaxFileSize != 0 {
			cfg.Rotation.MaxFileSize = rotation.MaxFileSize
		}
		if rotation.MaxTotalSize != 0 {
			cfg.Rotation.MaxTotalSize = rotation.MaxTotalSize
		}
	}
}

func WithFilecount(filecount types.Count) Option {
	return func(cfg *types.VLoggoConfig) { cfg.Filecount = filecount }
}

func WithDirectory(paths types.Paths) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.Directory = paths
		cfg.DirectoryTxtSet = paths.Txt != ""
		cfg.DirectoryJsonSet = paths.Json != ""
	}
}

func WithSMTP(smtp types.VLoggoSMTP) Option {
	return func(cfg *types.VLoggoConfig) {
		cfg.SMTP, cfg.SMTPSet = smtp, true
	}
}

func WithInput(input types.VLoggoConfigInput) Option {
	return func(cfg *types.VLoggoConfig) { ApplyInput(cfg, input) }
}

func applySMTPInput(smtp *types.VLoggoSMTP, input types.VLoggoSMTPInput) {
	if input.Host != nil {
		smtp.Host = *input.Host
	}
	if input.Port != nil {
		smtp.Port = *input.Port
		if input.Secure == nil && !smtp.SecureSet {
			smtp.Secure = false // re-infer from the new port during normalization
		}
	}
	if input.Username != nil {
		smtp.Username = *input.Username
	}
	if input.Password != nil {
		smtp.Password = *input.Password
	}
	if input.From != nil {
		smtp.From = *input.From
	}
	if input.To != nil {
		smtp.To = append([]string(nil), input.To...)
	}
	if input.Secure != nil {
		smtp.Secure, smtp.SecureSet = *input.Secure, true
	}
	if input.RequireTLS != nil {
		smtp.RequireTLS = *input.RequireTLS
	}
	if input.Debug != nil {
		smtp.Debug = *input.Debug
	}
	if input.ConnectionTimeout != nil {
		smtp.ConnectionTimeout, smtp.ConnectionTimeoutSet = *input.ConnectionTimeout, true
	}
	if input.GreetingTimeout != nil {
		smtp.GreetingTimeout, smtp.GreetingTimeoutSet = *input.GreetingTimeout, true
	}
	if input.SocketTimeout != nil {
		smtp.SocketTimeout, smtp.SocketTimeoutSet = *input.SocketTimeout, true
	}
}

func defaultClient() string {
	if client := os.Getenv("CLIENT_NAME"); strings.TrimSpace(client) != "" {
		return client
	}
	return DefaultClient
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func validLevel(level types.LogLevel) bool {
	switch level {
	case types.Debug, types.Info, types.Warn, types.Error, types.Fatal:
		return true
	default:
		return false
	}
}

func smtpEmpty(smtp types.VLoggoSMTP) bool {
	return smtp.Host == "" && smtp.Port == 0 && smtp.Username == "" && smtp.Password == "" && smtp.From == "" && len(smtp.To) == 0
}

func smtpEnvironmentPresent() bool {
	for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TO"} {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}
	return false
}

func smtpFromEnv() types.VLoggoSMTP {
	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	to := strings.Split(os.Getenv("SMTP_TO"), ",")
	if len(to) == 1 && strings.TrimSpace(to[0]) == "" {
		to = nil
	} else {
		for index := range to {
			to[index] = strings.TrimSpace(to[index])
		}
	}
	secure, _ := strconv.ParseBool(os.Getenv("SMTP_SECURE"))
	secureSet := os.Getenv("SMTP_SECURE") != ""
	if !secureSet {
		secure = port == 465
	}
	return types.VLoggoSMTP{
		Host: os.Getenv("SMTP_HOST"), Port: port,
		Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"),
		From: os.Getenv("SMTP_FROM"), To: to,
		Secure: secure, SecureSet: secureSet,
		ConnectionTimeout: DefaultNotificationTimeoutMS,
		GreetingTimeout:   DefaultNotificationTimeoutMS,
		SocketTimeout:     DefaultNotificationTimeoutMS,
	}
}
