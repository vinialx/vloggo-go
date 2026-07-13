package vloggo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	config "github.com/vinialx/vloggo-go/config"
	types "github.com/vinialx/vloggo-go/types"
)

func TestInfoWritesTxtAndJson(t *testing.T) {
	ClearInstances()
	t.Cleanup(ClearInstances)

	root := t.TempDir()
	logger := NewInstance("test-json",
		config.WithConsole(false),
		config.WithJSON(true),
		config.WithDirectory(types.Paths{
			Txt:  filepath.Join(root, "txt"),
			Json: filepath.Join(root, "json"),
		}),
	)

	done := make(chan struct{})
	go func() {
		logger.Info("BOOT", "service started")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("log write timed out")
	}

	txt := readOnlyLog(t, filepath.Join(root, "txt"), ".txt")
	if !strings.Contains(txt, "[INFO] [BOOT]") || !strings.Contains(txt, "service started") {
		t.Fatalf("txt log missing entry: %s", txt)
	}

	json := readOnlyLog(t, filepath.Join(root, "json"), ".jsonl")
	if !strings.Contains(json, `"level":"INFO"`) || !strings.Contains(json, `"code":"BOOT"`) {
		t.Fatalf("json log missing entry: %s", json)
	}
}

func TestDebugDisabledDoesNotWriteDebugEntry(t *testing.T) {
	ClearInstances()
	t.Cleanup(ClearInstances)

	root := t.TempDir()
	logger := NewInstance("test-debug",
		config.WithConsole(false),
		config.WithDebug(false),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "txt"), Json: filepath.Join(root, "json")}),
	)

	logger.Debug("DBG", "hidden")

	txt := readOnlyLog(t, filepath.Join(root, "txt"), ".txt")
	if strings.Contains(txt, "hidden") || strings.Contains(txt, "[DEBUG]") {
		t.Fatalf("debug entry was written while debug disabled: %s", txt)
	}
}

func TestCloneInitializesServices(t *testing.T) {
	ClearInstances()
	t.Cleanup(ClearInstances)

	root := t.TempDir()
	base := NewInstance("base",
		config.WithConsole(false),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "base-txt"), Json: filepath.Join(root, "base-json")}),
	)
	if base == nil {
		t.Fatal("base instance is nil")
	}

	clone := Clone("base", "child",
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "child-txt"), Json: filepath.Join(root, "child-json")}),
	)
	if clone == nil {
		t.Fatal("clone is nil")
	}

	clone.Info("CLONE", "clone works")
	txt := readOnlyLog(t, filepath.Join(root, "child-txt"), ".txt")
	if !strings.Contains(txt, "clone works") {
		t.Fatalf("clone log missing entry: %s", txt)
	}
}

func TestRemoteInputKeepsDefaults(t *testing.T) {
	ClearInstances()
	t.Cleanup(ClearInstances)

	root := t.TempDir()
	client := "remote"
	jsonEnabled := true
	txtDir := filepath.Join(root, "txt")
	jsonDir := filepath.Join(root, "json")

	logger := NewInstanceFromInput(types.VLoggoConfigInput{
		Client: &client,
		Json:   &jsonEnabled,
		Directory: &types.PathsInput{
			Txt:  &txtDir,
			Json: &jsonDir,
		},
	})

	cfg := logger.GetConfig()
	if !cfg.Console {
		t.Fatal("console default should remain enabled when omitted")
	}
	if cfg.Filecount.Txt != config.DefaultRetentionDay || cfg.Filecount.Json != config.DefaultRetentionDay {
		t.Fatalf("retention defaults not applied: %+v", cfg.Filecount)
	}
}

func TestRetentionDeletesOldDailyLogs(t *testing.T) {
	ClearInstances()
	t.Cleanup(ClearInstances)

	root := t.TempDir()
	txtDir := filepath.Join(root, "txt")
	jsonDir := filepath.Join(root, "json")
	if err := os.MkdirAll(txtDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"log-2020-01-01.txt", "log-2021-01-01.txt", "log-2022-01-01.txt"} {
		if err := os.WriteFile(filepath.Join(txtDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	NewInstance("retention",
		config.WithConsole(false),
		config.WithFilecount(types.Count{Txt: 2, Json: 2}),
		config.WithDirectory(types.Paths{Txt: txtDir, Json: jsonDir}),
	)

	if _, err := os.Stat(filepath.Join(txtDir, "log-2020-01-01.txt")); !os.IsNotExist(err) {
		t.Fatalf("oldest log should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(txtDir, "log-2021-01-01.txt")); !os.IsNotExist(err) {
		t.Fatalf("second oldest log should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(txtDir, "log-2022-01-01.txt")); err != nil {
		t.Fatalf("newest old log should be kept: %v", err)
	}
}

func readOnlyLog(t *testing.T, dir, ext string) string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ext {
			content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			return string(content)
		}
	}

	t.Fatalf("no %s log found in %s", ext, dir)
	return ""
}
