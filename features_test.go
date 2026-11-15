package vloggo

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinialx/vloggo-go/v2/config"
	services "github.com/vinialx/vloggo-go/v2/internal"
	"github.com/vinialx/vloggo-go/v2/types"
)

func TestIndependentOutputsAndConsoleOnlyMode(t *testing.T) {
	root := t.TempDir()
	logger := newFeatureLogger(t, "json-only",
		config.WithOutputs(types.VLoggoOutputs{JSON: true}),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "txt"), Json: filepath.Join(root, "json")}))
	logger.Info("JSON_ONLY", "structured event", types.LogData{"count": 3})
	if err := logger.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "txt")); !os.IsNotExist(err) {
		t.Fatalf("disabled text output created directory: %v", err)
	}
	entries := readJSONEntries(t, filepath.Join(root, "json"))
	if len(entries) != 1 || entries[0]["level"] != string(types.Info) {
		t.Fatalf("unexpected JSON-only events: %#v", entries)
	}

	consoleOnlyRoot := t.TempDir()
	consoleOnly := newFeatureLogger(t, "memory-only",
		config.WithOutputs(types.VLoggoOutputs{}),
		config.WithDirectory(types.Paths{Txt: filepath.Join(consoleOnlyRoot, "txt"), Json: filepath.Join(consoleOnlyRoot, "json")}))
	consoleOnly.Info("MEMORY_ONLY", "no file output")
	for _, outputDir := range []string{filepath.Join(consoleOnlyRoot, "txt"), filepath.Join(consoleOnlyRoot, "json")} {
		if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
			t.Fatalf("disabled file outputs created a directory: %v", err)
		}
	}
}

func TestChildContextStructuredJSONTextIDsAndRedaction(t *testing.T) {
	root := t.TempDir()
	logger := newFeatureLogger(t, "request-context",
		config.WithOutputs(types.VLoggoOutputs{Text: true, JSON: true}),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "txt"), Json: filepath.Join(root, "json")}),
		config.WithRedact("password", "headers.authorization", "users.*.token"))
	parent := logger.Child(types.LogData{"requestId": "req-17", "password": "secret"})
	child := parent.Child(types.LogData{
		"userId":  "usr-4",
		"headers": map[string]any{"authorization": "Bearer secret"},
		"users":   []any{map[string]any{"token": "hidden", "name": "Ada"}},
	})
	child.Info("HTTP_START", "Request received", types.LogData{"method": "POST", "path": "/login"})
	if err := logger.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := readJSONEntries(t, filepath.Join(root, "json"))
	if len(entries) != 1 {
		t.Fatalf("got %d JSON entries", len(entries))
	}
	entry := entries[0]
	if caller, ok := entry["caller"].(string); !ok || !strings.HasPrefix(caller, "features_test.go:") {
		t.Fatalf("caller did not point to logging call site: %#v", entry["caller"])
	}
	eventID, ok := entry["eventId"].(string)
	if !ok || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(eventID) {
		t.Fatalf("event ID is not a full UUID: %#v", entry["eventId"])
	}
	ctx := entry["context"].(map[string]any)
	if ctx["requestId"] != "req-17" || ctx["userId"] != "usr-4" || ctx["password"] != "[REDACTED]" {
		t.Fatalf("unexpected child context: %#v", ctx)
	}
	headers := ctx["headers"].(map[string]any)
	if headers["authorization"] != "[REDACTED]" {
		t.Fatalf("authorization was not redacted: %#v", headers)
	}
	users := ctx["users"].([]any)
	if users[0].(map[string]any)["token"] != "[REDACTED]" {
		t.Fatalf("wildcard redaction failed: %#v", users)
	}
	data := entry["data"].(map[string]any)
	if data["method"] != "POST" || data["path"] != "/login" {
		t.Fatalf("event data missing: %#v", data)
	}
	if entry["timestamp"] == "" || !strings.HasSuffix(entry["timestamp"].(string), "Z") {
		t.Fatalf("JSON timestamp is not UTC: %#v", entry["timestamp"])
	}
	text := readOnlyLog(t, filepath.Join(root, "txt"), ".txt")
	if !strings.Contains(text, "[evt-"+strings.ReplaceAll(eventID[:8], "-", "")+"]") || strings.Contains(text, "secret") || strings.Contains(text, "Bearer") {
		t.Fatalf("text ID or redaction incorrect: %s", text)
	}
	if strings.Count(text, "\n") != 1 || strings.Contains(text, "INIT") || strings.Contains(text, "____") {
		t.Fatalf("text output is not a single clean event: %q", text)
	}
}

func TestNativeErrorsCyclesSeverityAndAtomicUpdate(t *testing.T) {
	root := t.TempDir()
	logger := newFeatureLogger(t, "errors",
		config.WithOutputs(types.VLoggoOutputs{Text: true, JSON: true}),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "txt"), Json: filepath.Join(root, "json")}),
		config.WithLevel(types.Warn),
		config.WithRedact("error.message", "error.code", "error.cause.message", "password"))
	logger.Debug("HIDDEN", "filtered")
	logger.Info("HIDDEN", "filtered")
	logger.Warn("VISIBLE", "warning")

	inner := errors.New("inner secret")
	problem := &featureError{Message: "outer secret", Code: "DB-PRIVATE", Cause: inner}
	problem.Stack = "outer secret\n\tat service.go:9"
	logger.Error("DB_ERROR", "query failed", problem)
	cycle := map[string]any{"password": "hidden"}
	cycle["self"] = cycle
	logger.Error("CYCLE", "cyclic details", cycle)
	var nilError *featureError
	logger.Error("NIL_ERROR", "typed nil error is safe", nilError)
	if err := logger.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := readJSONEntries(t, filepath.Join(root, "json"))
	if len(entries) != 4 {
		t.Fatalf("expected warning plus three later events, got %#v", entries)
	}
	serializedError := entries[1]["error"].(map[string]any)
	if serializedError["message"] != "[REDACTED]" || serializedError["Code"] != "[REDACTED]" {
		t.Fatalf("native error fields were not redacted: %#v", serializedError)
	}
	if strings.Contains(serializedError["stack"].(string), "outer secret") {
		t.Fatalf("redacted error message leaked through stack: %#v", serializedError["stack"])
	}
	if serializedError["cause"].(map[string]any)["message"] != "[REDACTED]" {
		t.Fatalf("wrapped cause was not serialized/redacted: %#v", serializedError)
	}
	cycleData := entries[2]["data"].(map[string]any)
	if cycleData["password"] != "[REDACTED]" || cycleData["self"] != "[Circular]" {
		t.Fatalf("cycle or secret handling failed: %#v", cycleData)
	}
	if entries[3]["error"] != nil || entries[3]["data"] != nil {
		t.Fatalf("typed nil error produced data or panicked: %#v", entries[3])
	}
	if strings.Contains(readOnlyLog(t, filepath.Join(root, "txt"), ".txt"), "outer secret") {
		t.Fatal("text output leaked native error secret")
	}

	before := logger.GetConfig()
	if err := logger.Update(config.WithLevel(types.LogLevel("TRACE"))); err == nil {
		t.Fatal("invalid update should fail")
	}
	after := logger.GetConfig()
	if before.Level != after.Level || before.Outputs != after.Outputs {
		t.Fatalf("invalid update changed live config: before=%+v after=%+v", before, after)
	}
}

func TestSizeRotationRetentionAndSymlinkSafety(t *testing.T) {
	root := t.TempDir()
	txtDir := filepath.Join(root, "logs")
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("leave unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(txtDir, 0700); err != nil {
		t.Fatal(err)
	}
	date := time.Now().Format("2006-01-02")
	if err := os.Symlink(outside, filepath.Join(txtDir, "log-"+date+".txt")); err != nil {
		t.Fatal(err)
	}
	logger := newFeatureLogger(t, "rotation",
		config.WithOutputs(types.VLoggoOutputs{Text: true}),
		config.WithDirectory(types.Paths{Txt: txtDir, Json: filepath.Join(root, "json")}),
		config.WithFilecount(types.Count{Txt: 100, Json: 100}),
		config.WithRotation(types.VLoggoRotation{RetentionDays: 1000, MaxFileSize: 260, MaxTotalSize: 520}))
	for index := 0; index < 8; index++ {
		logger.Info(fmt.Sprintf("EVENT_%d", index), strings.Repeat("x", 80))
	}
	if err := logger.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "leave unchanged" {
		t.Fatalf("logger followed symlink: content=%q err=%v", content, err)
	}
	entries, err := os.ReadDir(txtDir)
	if err != nil {
		t.Fatal(err)
	}
	segments := 0
	var total int64
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "log-") || !strings.HasSuffix(entry.Name(), ".txt") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		segments++
		total += info.Size()
		if info.Size() > 260 {
			t.Fatalf("segment exceeded max file size: %s=%d", entry.Name(), info.Size())
		}
	}
	if segments < 2 || total > 520 {
		t.Fatalf("size rotation/total retention failed: segments=%d total=%d", segments, total)
	}
}

func TestTimezoneFormattingCloseAndFatalDoesNotExit(t *testing.T) {
	format := services.NewFormatService("api", "Pacific/Auckland")
	date := time.Date(2026, time.January, 1, 12, 0, 0, 123000000, time.UTC)
	if format.Day(date) != "2026-01-02" || format.Date(date) != "02/01/2026 01:00:00.123" {
		t.Fatalf("timezone format incorrect: day=%s date=%s", format.Day(date), format.Date(date))
	}
	if format.IsoDate(date) != "2026-01-01T12:00:00.123Z" {
		t.Fatalf("JSON timestamp is not fixed UTC milliseconds: %s", format.IsoDate(date))
	}

	root := t.TempDir()
	logger := newFeatureLogger(t, "close",
		config.WithOutputs(types.VLoggoOutputs{Text: true}),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "txt"), Json: filepath.Join(root, "json")}))
	logger.Fatal("STILL_RUNNING", "Fatal must not exit the process")
	if content := readOnlyLog(t, filepath.Join(root, "txt"), ".txt"); !strings.Contains(content, "STILL_RUNNING") {
		t.Fatalf("Fatal did not write event: %s", content)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := logger.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(ctx); err != nil {
		t.Fatalf("repeated Close failed: %v", err)
	}
	before := readOnlyLog(t, filepath.Join(root, "txt"), ".txt")
	logger.Info("AFTER_CLOSE", "must be ignored")
	if after := readOnlyLog(t, filepath.Join(root, "txt"), ".txt"); after != before {
		t.Fatalf("closed logger accepted event: before=%q after=%q", before, after)
	}
}

func TestSMTPQuietInvalidDiagnosticsEscapingAndFullEventID(t *testing.T) {
	for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TO", "SMTP_SECURE"} {
		t.Setenv(key, "")
	}
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	logger := newFeatureLogger(t, "smtp-invalid",
		config.WithOutputs(types.VLoggoOutputs{Console: true}),
		config.WithNotify(true))
	logger.Fatal("MISSING_SMTP", "continues to log")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = oldStderr
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	captured := string(output)
	if count := strings.Count(captured, "[SMTP_CONFIG]"); count != 1 {
		t.Fatalf("expected one SMTP diagnostic, got %d: %s", count, captured)
	}
	if !strings.Contains(captured, "MISSING_SMTP") {
		t.Fatal("invalid SMTP configuration stopped normal logging")
	}
	_ = logger.Close(context.Background())

	server := newFakeSMTP(t, 0, false)
	root := t.TempDir()
	valid := newFeatureLogger(t, "smtp-valid",
		config.WithOutputs(types.VLoggoOutputs{}),
		config.WithDirectory(types.Paths{Txt: filepath.Join(root, "txt"), Json: filepath.Join(root, "json")}),
		config.WithNotify(true), config.WithThrottle(0),
		config.WithSMTP(types.VLoggoSMTP{
			Host: "127.0.0.1", Port: server.port(), Username: "user", Password: "password",
			From: `Logs <logger@example.test>`, To: []string{"ops@example.test"},
			ConnectionTimeout: 500, GreetingTimeout: 500, SocketTimeout: 500,
		}),
		config.WithRedact("password"))

	valid.Fatal("<SCRIPT>", "<img src=x onerror=alert(1)>", types.LogData{"password": "do-not-send", "requestId": "req-9"})
	message := <-server.messages
	if !strings.Contains(message, "From: ") || !strings.Contains(message, `"Logs" <logger@example.test>`) || !strings.Contains(message, "Content-Type: text/plain") || !strings.Contains(message, "Content-Type: text/html") {
		t.Fatalf("SMTP message lacks structured sender or alternatives: %s", message)
	}
	htmlMessage := strings.SplitN(message, "Content-Type: text/html; charset=UTF-8", 2)
	if len(htmlMessage) != 2 || !strings.Contains(htmlMessage[1], "&lt;img src=x onerror=alert(1)&gt;") || strings.Contains(htmlMessage[1], "<p><img src=x") {
		t.Fatalf("email HTML was not escaped: %s", message)
	}
	if !strings.Contains(message, "[REDACTED]") || strings.Contains(message, "do-not-send") || !strings.Contains(message, "req-9") {
		t.Fatalf("email details were not redacted/correlated: %s", message)
	}
	if !regexp.MustCompile(`Event ID: [0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`).MatchString(message) {
		t.Fatalf("email does not include full event UUID: %s", message)
	}
}

func TestSMTPThrottlesConcurrentDeliveryAndRetriesAfterFailure(t *testing.T) {
	server := newFakeSMTP(t, 120*time.Millisecond, false)
	logger := newFeatureLogger(t, "smtp-concurrent",
		config.WithOutputs(types.VLoggoOutputs{}),
		config.WithNotify(true), config.WithThrottle(5000),
		config.WithSMTP(fakeSMTPConfig(server.port())))
	finished := make(chan struct{}, 2)
	go func() { logger.Fatal("ONE", "first"); finished <- struct{}{} }()
	go func() { logger.Fatal("TWO", "second"); finished <- struct{}{} }()
	<-finished
	<-finished
	if got := server.deliveries.Load(); got != 1 {
		t.Fatalf("concurrent fatals sent %d emails; want one", got)
	}
	if err := logger.Update(config.WithLevel(types.Warn)); err != nil {
		t.Fatal(err)
	}
	logger.Fatal("AFTER_UPDATE", "successful delivery remains throttled")
	if got := server.deliveries.Load(); got != 1 {
		t.Fatalf("configuration update reset successful-send throttle: got %d deliveries", got)
	}
	_ = logger.Close(context.Background())

	retrying := newFakeSMTP(t, 0, true)
	logger = newFeatureLogger(t, "smtp-retry",
		config.WithOutputs(types.VLoggoOutputs{}), config.WithNotify(true),
		config.WithThrottle(0), config.WithRetryDelay(80),
		config.WithSMTP(fakeSMTPConfig(retrying.port())))
	if err := logger.FatalContext(context.Background(), "FAIL_FIRST", "temporary fail"); err == nil {
		t.Fatal("first SMTP failure was not returned")
	}
	if err := logger.FatalContext(context.Background(), "TOO_SOON", "suppressed"); err != nil {
		t.Fatalf("retry throttle should suppress without error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := logger.FatalContext(context.Background(), "RETRY", "send again"); err != nil {
		t.Fatalf("retry after delay failed: %v", err)
	}
	if got := retrying.deliveries.Load(); got != 2 {
		t.Fatalf("got %d SMTP delivery attempts; want 2", got)
	}
	_ = logger.Close(context.Background())
}

func TestSMTPUpdatePreservesInFlightGuard(t *testing.T) {
	server := newFakeSMTP(t, 150*time.Millisecond, false)
	logger := newFeatureLogger(t, "smtp-update-flight",
		config.WithOutputs(types.VLoggoOutputs{}), config.WithNotify(true),
		config.WithThrottle(0), config.WithSMTP(fakeSMTPConfig(server.port())))
	finished := make(chan struct{})
	go func() {
		logger.Fatal("FIRST", "delivery in flight")
		close(finished)
	}()
	deadline := time.Now().Add(time.Second)
	for server.deliveries.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if server.deliveries.Load() == 0 {
		t.Fatal("first SMTP attempt did not reach the server")
	}
	if err := logger.Update(config.WithLevel(types.Debug)); err != nil {
		t.Fatal(err)
	}
	logger.Fatal("SECOND", "must be suppressed while first is active")
	<-finished
	if got := server.deliveries.Load(); got != 1 {
		t.Fatalf("update allowed overlapping SMTP attempt: got %d deliveries", got)
	}
	_ = logger.Close(context.Background())
}

func TestSMTPNotificationTimeoutAndRequireTLS(t *testing.T) {
	server := newFakeSMTP(t, 300*time.Millisecond, false)
	logger := newFeatureLogger(t, "smtp-timeout",
		config.WithOutputs(types.VLoggoOutputs{}), config.WithNotify(true),
		config.WithNotificationTimeout(50), config.WithThrottle(0),
		config.WithSMTP(fakeSMTPConfig(server.port())))
	started := time.Now()
	err := logger.FatalContext(context.Background(), "TIMEOUT", "server delays response")
	if err == nil {
		t.Fatal("expected notification timeout error")
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Fatalf("notification timeout took %s; want under 250ms", elapsed)
	}
	_ = logger.Close(context.Background())

	server = newFakeSMTP(t, 0, false)
	smtp := fakeSMTPConfig(server.port())
	smtp.RequireTLS = true
	logger = newFeatureLogger(t, "smtp-require-tls",
		config.WithOutputs(types.VLoggoOutputs{}), config.WithNotify(true),
		config.WithThrottle(0), config.WithSMTP(smtp))
	err = logger.FatalContext(context.Background(), "TLS_REQUIRED", "plain connection rejected")
	if err == nil || !strings.Contains(err.Error(), "required STARTTLS") {
		t.Fatalf("required STARTTLS was not enforced: %v", err)
	}
	_ = logger.Close(context.Background())
}

func TestClientRenameKeepsRegistryAndDefaultDirectoriesConsistent(t *testing.T) {
	logger := newFeatureLogger(t, "rename-source", config.WithOutputs(types.VLoggoOutputs{}))
	if _, err := NewInstanceE("rename-target", config.WithOutputs(types.VLoggoOutputs{})); err != nil {
		t.Fatal(err)
	}
	if err := logger.Update(config.WithClient("rename-target")); err == nil {
		t.Fatal("client rename to an existing logger should fail")
	}
	if got := logger.GetConfig().Client; got != "rename-source" {
		t.Fatalf("failed rename changed client: %q", got)
	}

	if err := logger.Update(config.WithClient("renamed")); err != nil {
		t.Fatal(err)
	}
	instances := GetAllInstances()
	if instances["renamed"] != logger || instances["rename-source"] != nil {
		t.Fatalf("registry did not follow rename: %#v", instances)
	}
	paths := logger.GetConfig().Directory
	want := config.DefaultDirectory("renamed")
	if paths != want {
		t.Fatalf("default directories did not follow client: got %+v want %+v", paths, want)
	}
	clone := Clone("renamed", "renamed-clone")
	if clone == nil || clone.GetConfig().Directory != config.DefaultDirectory("renamed-clone") {
		t.Fatalf("clone did not receive client-specific directories: %#v", clone)
	}
}

func newFeatureLogger(t *testing.T, client string, options ...config.Option) *VLoggo {
	t.Helper()
	ClearInstances()
	t.Cleanup(ClearInstances)
	logger, err := NewInstanceE(client, options...)
	if err != nil {
		t.Fatal(err)
	}
	return logger
}

func readJSONEntries(t *testing.T, directory string) []map[string]any {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
			if line == "" {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("invalid JSONL line %q: %v", line, err)
			}
			records = append(records, record)
		}
	}
	return records
}

type featureError struct {
	Message string
	Code    string
	Cause   error
	Stack   string
}

func (e *featureError) Error() string      { return e.Message }
func (e *featureError) Unwrap() error      { return e.Cause }
func (e *featureError) StackTrace() string { return e.Stack }

type fakeSMTP struct {
	listener   net.Listener
	delay      time.Duration
	failFirst  atomic.Bool
	deliveries atomic.Int32
	messages   chan string
}

func newFakeSMTP(t *testing.T, delay time.Duration, failFirst bool) *fakeSMTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &fakeSMTP{listener: listener, delay: delay, messages: make(chan string, 8)}
	server.failFirst.Store(failFirst)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go server.handle(connection)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return server
}

func (server *fakeSMTP) port() int {
	return server.listener.Addr().(*net.TCPAddr).Port
}

func (server *fakeSMTP) handle(connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	write := func(value string) bool {
		if _, err := fmt.Fprint(writer, value+"\r\n"); err != nil {
			return false
		}
		return writer.Flush() == nil
	}
	if !write("220 test SMTP ready") {
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.Fields(strings.TrimSpace(line))[0])
		switch command {
		case "EHLO", "HELO":
			if !write("250-localhost") || !write("250 SIZE 10485760") {
				return
			}
		case "AUTH":
			if !write("235 authentication accepted") {
				return
			}
		case "MAIL", "RCPT", "RSET", "NOOP":
			if !write("250 command accepted") {
				return
			}
		case "DATA":
			if !write("354 end with dot") {
				return
			}
			var body strings.Builder
			for {
				dataLine, readErr := reader.ReadString('\n')
				if readErr != nil {
					return
				}
				if dataLine == ".\r\n" {
					break
				}
				if strings.HasPrefix(dataLine, "..") {
					dataLine = dataLine[1:]
				}
				body.WriteString(dataLine)
			}
			server.deliveries.Add(1)
			if server.delay > 0 {
				time.Sleep(server.delay)
			}
			server.messages <- body.String()
			if server.failFirst.CompareAndSwap(true, false) {
				if !write("451 temporary error") {
					return
				}
			} else if !write("250 message accepted") {
				return
			}
		case "QUIT":
			write("221 bye")
			return
		default:
			if !write("250 command accepted") {
				return
			}
		}
	}
}

func fakeSMTPConfig(port int) types.VLoggoSMTP {
	return types.VLoggoSMTP{
		Host: "127.0.0.1", Port: port, Username: "user", Password: "password",
		From: "logs@example.test", To: []string{"ops@example.test"},
		ConnectionTimeout: 500, GreetingTimeout: 500, SocketTimeout: 500,
	}
}
