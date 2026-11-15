package config

import (
	"testing"

	"github.com/vinialx/vloggo-go/v2/types"
)

func TestDefaultsLegacyAliasesAndPartialOutputs(t *testing.T) {
	t.Setenv("CLIENT_NAME", "")
	cfg := Normalize(DefaultConfig("api"))
	if cfg.Level != types.Info || !cfg.Outputs.Console || !cfg.Outputs.Text || cfg.Outputs.JSON {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.Filecount.Txt != 31 || cfg.Filecount.Json != 31 || cfg.Rotation.RetentionDays != 31 {
		t.Fatalf("retention defaults missing: %+v", cfg)
	}

	WithDebug(true)(&cfg)
	cfg = Normalize(cfg)
	if cfg.Level != types.Debug {
		t.Fatalf("legacy debug option did not set debug level: %q", cfg.Level)
	}
	WithLevel(types.Warn)(&cfg)
	WithDebug(false)(&cfg)
	cfg = Normalize(cfg)
	if cfg.Level != types.Warn {
		t.Fatalf("explicit level lost precedence over debug alias: %q", cfg.Level)
	}

	jsonEnabled, consoleDisabled := true, false
	input := types.VLoggoConfigInput{Outputs: &types.VLoggoOutputsInput{JSON: &jsonEnabled, Console: &consoleDisabled}}
	partial := FromInput(input)
	if partial.Outputs.Console || !partial.Outputs.Text || !partial.Outputs.JSON {
		t.Fatalf("partial output input did not retain defaults: %+v", partial.Outputs)
	}
}

func TestNotifyFalseSkipsEnvironmentAndNotifyTrueLoadsIt(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "logs@example.test")
	t.Setenv("SMTP_TO", "ops@example.test")

	disabled := DefaultConfig("disabled")
	WithNotify(false)(&disabled)
	disabled, err := Resolve(disabled)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Notify || !smtpEmpty(disabled.SMTP) {
		t.Fatalf("notify=false loaded SMTP environment: %+v", disabled.SMTP)
	}

	enabled := DefaultConfig("enabled")
	WithNotify(true)(&enabled)
	enabled, err = Resolve(enabled)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Notify || enabled.SMTP.Host != "smtp.example.test" || !enabled.SMTP.Secure {
		t.Fatalf("notify=true did not load and normalize SMTP environment: %+v", enabled)
	}
}

func TestPartialSMTPEnvironmentStaysQuietUnlessRequired(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_TO", "")

	optional, err := Resolve(DefaultConfig("optional"))
	if err != nil {
		t.Fatal(err)
	}
	if optional.Notify || optional.NotificationIssue != "" {
		t.Fatalf("partial optional SMTP configuration was not silent: %+v", optional)
	}

	required := DefaultConfig("required")
	WithNotify(true)(&required)
	required, err = Resolve(required)
	if err != nil {
		t.Fatal(err)
	}
	if !required.Notify || required.NotificationIssue == "" {
		t.Fatalf("explicit notification requirement did not record missing SMTP fields: %+v", required)
	}
}

func TestValidationRejectsInvalidOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*types.VLoggoConfig)
	}{
		{"negative throttle", func(cfg *types.VLoggoConfig) { cfg.Throttle = -1 }},
		{"invalid timezone", func(cfg *types.VLoggoConfig) { cfg.Format.Timezone = "Mars/Olympus" }},
		{"invalid level", func(cfg *types.VLoggoConfig) { cfg.Level = "TRACE"; cfg.LevelSet = true }},
		{"invalid file size", func(cfg *types.VLoggoConfig) { cfg.Rotation.MaxFileSize = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultConfig("invalid")
			test.edit(&cfg)
			if _, err := Resolve(cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestInputPreservesExplicitZeroAndEmptyValuesForValidation(t *testing.T) {
	zero := 0
	empty := ""
	for _, test := range []struct {
		name  string
		input types.VLoggoConfigInput
	}{
		{"zero filecount", types.VLoggoConfigInput{Filecount: &types.CountInput{Txt: &zero}}},
		{"zero rotation size", types.VLoggoConfigInput{Rotation: &types.VLoggoRotationInput{MaxFileSize: int64Pointer(0)}}},
		{"empty timezone", types.VLoggoConfigInput{Format: &types.VLoggoFormatInput{Timezone: &empty}}},
		{"empty directory", types.VLoggoConfigInput{Directory: &types.PathsInput{Txt: &empty}}},
		{"empty client", types.VLoggoConfigInput{Client: &empty}},
		{"zero SMTP timeout", types.VLoggoConfigInput{SMTP: &types.VLoggoSMTPInput{ConnectionTimeout: &zero}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Resolve(FromInput(test.input)); err == nil {
				t.Fatal("expected explicit invalid value to fail validation")
			}
		})
	}
}

func TestClientNameEnvironmentDefault(t *testing.T) {
	t.Setenv("CLIENT_NAME", "worker")
	if got := DefaultConfig().Client; got != "worker" {
		t.Fatalf("CLIENT_NAME default = %q; want worker", got)
	}
	if got := DefaultConfig("explicit").Client; got != "explicit" {
		t.Fatalf("explicit client was overridden by environment: %q", got)
	}
}

func int64Pointer(value int64) *int64 { return &value }
