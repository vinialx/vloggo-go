package vloggo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinialx/vloggo-go/v2/config"
	services "github.com/vinialx/vloggo-go/v2/internal"
	"github.com/vinialx/vloggo-go/v2/types"
)

type loggerState struct {
	mu           sync.Mutex
	cfg          types.VLoggoConfig
	file         *services.FileService
	fileGen      *fileGeneration
	format       *services.FormatService
	email        *services.EmailService
	closed       bool
	closedFlag   atomic.Bool // mirrors closed for readers holding instancesMu
	inflight     int
	idle         chan struct{}
	closeStarted bool
	closeDone    chan struct{}
	closeErr     error
	reportedSMTP string
}

type fileGeneration struct {
	service *services.FileService
	pending sync.WaitGroup
}

// VLoggo logger supports child contexts while sharing outputs and lifecycle.
type VLoggo struct {
	state   *loggerState
	context types.LogData
}

var (
	instancesMu sync.RWMutex
	instances   = make(map[string]*VLoggo)
)

func NewInstance(client string, opts ...config.Option) *VLoggo {
	logger, err := NewInstanceE(client, opts...)
	if err == nil {
		return logger
	}
	fallback := config.DefaultConfig()
	diagnostic(fallback, "CONFIG", fmt.Sprintf("invalid logger configuration: %v", err))
	return newVLoggo(fallback)
}

// NewInstanceE creates or returns a named logger and reports invalid configuration.
func NewInstanceE(client string, opts ...config.Option) (*VLoggo, error) {
	cfg := config.DefaultConfig(client)
	baseClient := cfg.Client
	for _, option := range opts {
		if option != nil {
			option(&cfg)
		}
	}
	rebaseDefaultDirectories(&cfg, baseClient)
	resolved, err := config.Resolve(cfg)
	if err != nil {
		return nil, err
	}
	return instanceFor(resolved), nil
}

func NewInstanceFromConfig(cfg types.VLoggoConfig) *VLoggo {
	logger, err := NewInstanceFromConfigE(cfg)
	if err == nil {
		return logger
	}
	fallback := config.DefaultConfig()
	diagnostic(fallback, "CONFIG", fmt.Sprintf("invalid logger configuration: %v", err))
	return newVLoggo(fallback)
}

// NewInstanceFromConfigE validates a complete configuration before creating a logger.
func NewInstanceFromConfigE(cfg types.VLoggoConfig) (*VLoggo, error) {
	resolved, err := config.Resolve(cloneConfig(cfg))
	if err != nil {
		return nil, err
	}
	return instanceFor(resolved), nil
}

func NewInstanceFromInput(input types.VLoggoConfigInput) *VLoggo {
	return NewInstanceFromConfig(config.FromInput(input))
}

func NewInstanceFromInputE(input types.VLoggoConfigInput) (*VLoggo, error) {
	return NewInstanceFromConfigE(config.FromInput(input))
}

func instanceFor(cfg types.VLoggoConfig) *VLoggo {
	instancesMu.Lock()
	defer instancesMu.Unlock()
	if instance, exists := instances[cfg.Client]; exists && !instance.isClosed() {
		return instance
	}
	instance := newVLoggo(cfg)
	instances[cfg.Client] = instance
	return instance
}

func newVLoggo(cfg types.VLoggoConfig) *VLoggo {
	cfg = cloneConfig(config.Normalize(cfg))
	state := &loggerState{
		cfg:    cfg,
		file:   services.NewFileService(cfg),
		format: services.NewFormatService(cfg.Client, cfg.Format.Timezone),
		email:  services.NewEmailService(cfg),
	}
	state.fileGen = &fileGeneration{service: state.file}
	logger := &VLoggo{state: state}
	if err := state.file.Initialize(); err != nil {
		diagnostic(cfg, "FILE_INIT", err.Error())
	}
	state.reportSMTPIssue()
	if cfg.StartupMessage {
		logger.Info("APP_START", "Logger initialized")
	}
	return logger
}

func (v *VLoggo) isClosed() bool {
	return v.state.closedFlag.Load()
}

func GetAllInstances() map[string]*VLoggo {
	instancesMu.RLock()
	defer instancesMu.RUnlock()
	result := make(map[string]*VLoggo, len(instances))
	for client, instance := range instances {
		result[client] = instance
	}
	return result
}

// GetConfig returns an independent copy of the current normalized configuration.
func (v *VLoggo) GetConfig() types.VLoggoConfig {
	if v == nil || v.state == nil {
		return config.DefaultConfig()
	}
	v.state.mu.Lock()
	defer v.state.mu.Unlock()
	return cloneConfig(v.state.cfg)
}

func RemoveInstance(client string) bool {
	instancesMu.Lock()
	instance, exists := instances[client]
	if exists {
		delete(instances, client)
	}
	instancesMu.Unlock()
	if exists {
		_ = instance.Close(context.Background())
	}
	return exists
}

func ClearInstances() {
	instancesMu.Lock()
	removed := instances
	instances = make(map[string]*VLoggo)
	instancesMu.Unlock()
	for _, instance := range removed {
		_ = instance.Close(context.Background())
	}
}

func Clone(base, client string, opts ...config.Option) *VLoggo {
	instancesMu.RLock()
	baseInstance := instances[base]
	instancesMu.RUnlock()
	if baseInstance == nil {
		return nil
	}
	cfg := baseInstance.GetConfig()
	baseClient := cfg.Client
	cfg.Client = client
	for _, option := range opts {
		if option != nil {
			option(&cfg)
		}
	}
	rebaseDefaultDirectories(&cfg, baseClient)
	return NewInstanceFromConfig(cfg)
}

// Update applies options atomically. Invalid changes leave the active logger unchanged.
func (v *VLoggo) Update(opts ...config.Option) error {
	if v == nil || v.state == nil {
		return errors.New("logger is not initialized")
	}
	state := v.state
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		return errors.New("logger is closed")
	}
	next := cloneConfig(state.cfg)
	oldClient := state.cfg.Client
	for _, option := range opts {
		if option != nil {
			option(&next)
		}
	}
	rebaseDefaultDirectories(&next, oldClient)
	resolved, err := config.Resolve(next)
	if err != nil {
		state.mu.Unlock()
		return err
	}
	newFile := services.NewFileService(resolved)
	newFileGen := &fileGeneration{service: newFile}
	newFormat := services.NewFormatService(resolved.Client, resolved.Format.Timezone)
	instancesMu.Lock()
	if resolved.Client != oldClient {
		if existing := instances[resolved.Client]; existing != nil && existing.state != state {
			instancesMu.Unlock()
			state.mu.Unlock()
			return fmt.Errorf("logger client %q already exists", resolved.Client)
		}
	}
	if initErr := newFile.Initialize(); initErr != nil {
		diagnostic(resolved, "FILE_INIT", initErr.Error())
	}
	if state.email == nil {
		state.email = services.NewEmailService(resolved)
	} else if err := state.email.UpdateConfig(resolved); err != nil {
		instancesMu.Unlock()
		state.mu.Unlock()
		_ = newFile.Close()
		return err
	}
	oldFileGen := state.fileGen
	state.cfg, state.file, state.fileGen, state.format = cloneConfig(resolved), newFile, newFileGen, newFormat
	if resolved.Client != oldClient {
		if root := instances[oldClient]; root != nil && root.state == state {
			delete(instances, oldClient)
			instances[resolved.Client] = root
		}
	}
	instancesMu.Unlock()
	state.mu.Unlock()
	if oldFileGen != nil {
		go func() {
			oldFileGen.pending.Wait()
			_ = oldFileGen.service.Close()
		}()
	}
	state.reportSMTPIssue()
	return nil
}

// Child returns a logger with copied context and shared configuration, outputs, and lifecycle.
func (v *VLoggo) Child(values types.LogData) *VLoggo {
	if v == nil || v.state == nil {
		return &VLoggo{}
	}
	redact := v.GetConfig().Redact
	childContext := services.SanitizeData(values, redact)
	merged := cloneData(v.context)
	for key, value := range childContext {
		merged[key] = value
	}
	return &VLoggo{state: v.state, context: merged}
}

func (v *VLoggo) log(ctx context.Context, level types.LogLevel, code, message string, metadata ...any) error {
	if v == nil || v.state == nil {
		return errors.New("logger is not initialized")
	}
	state := v.state
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		return errors.New("logger is closed")
	}
	cfg := cloneConfig(state.cfg)
	if severity(level) < severity(cfg.Level) {
		state.mu.Unlock()
		return nil
	}
	file, fileGen, format, email := state.file, state.fileGen, state.format, state.email
	contextValues := cloneData(v.context)
	state.beginLocked()
	if fileGen != nil {
		fileGen.pending.Add(1)
	}
	state.mu.Unlock()
	defer state.end()
	if fileGen != nil {
		defer fileGen.pending.Done()
	}

	if strings.TrimSpace(code) == "" {
		return errors.New("log code must be nonempty")
	}
	now := time.Now()
	data, nativeError := collectMetadata(metadata, cfg.Redact)
	contextData := services.SanitizeData(contextValues, cfg.Redact)
	if len(contextData) == 0 {
		contextData = nil
	}
	if len(data) == 0 {
		data = nil
	}
	entry := types.LogEntry{
		EventID:   services.NewEventID(),
		Client:    cfg.Client,
		Level:     level,
		Timestamp: format.IsoDate(now),
		Code:      code,
		Caller:    services.Caller(0),
		Message:   message,
		Context:   contextData,
		Data:      data,
	}
	if nativeError != nil {
		entry.Error = services.SerializeError(nativeError, cfg.Redact)
	}
	line := format.Line(entry)
	if cfg.Outputs.Console {
		writer := os.Stdout
		if level == types.Warn || level == types.Error || level == types.Fatal {
			writer = os.Stderr
		}
		_, _ = fmt.Fprint(writer, line)
	}
	if file != nil {
		if err := file.Write(entry); err != nil {
			diagnostic(cfg, "FILE_WRITE", err.Error())
		}
	}
	if level == types.Fatal && cfg.Notify && email != nil {
		if err := email.NotifyFatalContext(ctx, entry); err != nil {
			diagnostic(cfg, "SMTP_SEND", err.Error())
			return err
		}
	}
	return nil
}

func (v *VLoggo) Debug(code, message string, metadata ...any) {
	_ = v.log(context.Background(), types.Debug, code, message, metadata...)
}

func (v *VLoggo) Info(code, message string, metadata ...any) {
	_ = v.log(context.Background(), types.Info, code, message, metadata...)
}

func (v *VLoggo) Warn(code, message string, metadata ...any) {
	_ = v.log(context.Background(), types.Warn, code, message, metadata...)
}

func (v *VLoggo) Error(code, message string, metadata ...any) {
	_ = v.log(context.Background(), types.Error, code, message, metadata...)
}

// Fatal logs and waits for any enabled notification. It never exits the process.
func (v *VLoggo) Fatal(code, message string, metadata ...any) {
	_ = v.log(context.Background(), types.Fatal, code, message, metadata...)
}

// FatalContext allows callers to bound the fatal notification attempt.
func (v *VLoggo) FatalContext(ctx context.Context, code, message string, metadata ...any) error {
	return v.log(ctx, types.Fatal, code, message, metadata...)
}

// Flush waits for logging work already accepted by the logger and applies retention cleanup.
func (v *VLoggo) Flush(ctx context.Context) error {
	if v == nil || v.state == nil {
		return errors.New("logger is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state := v.state
	state.mu.Lock()
	done := state.idleLocked()
	state.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	state.mu.Lock()
	file := state.file
	state.mu.Unlock()
	if file != nil {
		return file.Flush()
	}
	return nil
}

// Close rejects new events, waits for in-flight work, and releases active files.
func (v *VLoggo) Close(ctx context.Context) error {
	if v == nil || v.state == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state := v.state
	state.mu.Lock()
	if !state.closeStarted {
		state.closed, state.closeStarted = true, true
		state.closedFlag.Store(true)
		state.closeDone = make(chan struct{})
		file, email := state.file, state.email
		idle := state.idleLocked()
		go func() {
			<-idle
			var failures []error
			if email != nil {
				if err := email.Close(); err != nil {
					failures = append(failures, err)
				}
			}
			if file != nil {
				if err := file.Close(); err != nil {
					failures = append(failures, err)
				}
			}
			state.mu.Lock()
			state.closeErr = errors.Join(failures...)
			close(state.closeDone)
			state.mu.Unlock()
		}()
	}
	done := state.closeDone
	state.mu.Unlock()
	select {
	case <-done:
		state.mu.Lock()
		err := state.closeErr
		state.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// beginLocked registers one in-flight event. The caller must hold state.mu.
func (state *loggerState) beginLocked() {
	if state.inflight == 0 {
		state.idle = make(chan struct{})
	}
	state.inflight++
}

func (state *loggerState) end() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.inflight--
	if state.inflight == 0 {
		close(state.idle)
	}
}

// idleLocked returns a channel closed once no events are in flight. The caller must hold state.mu.
func (state *loggerState) idleLocked() <-chan struct{} {
	if state.inflight == 0 {
		done := make(chan struct{})
		close(done)
		return done
	}
	return state.idle
}

func (state *loggerState) reportSMTPIssue() {
	state.mu.Lock()
	issue := state.cfg.NotificationIssue
	if issue == "" && state.email != nil {
		if err := state.email.ConfigurationIssue(); err != nil {
			issue = err.Error()
		}
	}
	if !state.cfg.Notify {
		state.reportedSMTP = ""
		state.mu.Unlock()
		return
	}
	if issue == "" {
		state.reportedSMTP = ""
		state.mu.Unlock()
		return
	}
	if issue == state.reportedSMTP || !state.cfg.Outputs.Console {
		state.mu.Unlock()
		return
	}
	cfg := cloneConfig(state.cfg)
	state.reportedSMTP = issue
	state.mu.Unlock()
	diagnostic(cfg, "SMTP_CONFIG", "notifications enabled but "+issue)
}

func diagnostic(cfg types.VLoggoConfig, code, message string) {
	if !cfg.Outputs.Console {
		return
	}
	format := services.NewFormatService(cfg.Client, cfg.Format.Timezone)
	id := services.NewEventID()
	entry := types.LogEntry{
		EventID: id, Client: cfg.Client, Level: types.Error,
		Timestamp: format.IsoDate(), Code: code, Caller: "(internal)", Message: message,
	}
	_, _ = fmt.Fprint(os.Stderr, format.Line(entry))
}

func collectMetadata(values []any, patterns []string) (types.LogData, error) {
	data := make(types.LogData)
	var eventError error
	for _, value := range values {
		if value == nil {
			continue
		}
		if err, ok := value.(error); ok {
			if eventError == nil {
				eventError = err
			}
			continue
		}
		sanitized := services.Sanitize(value, patterns)
		if fields, ok := sanitized.(map[string]any); ok {
			for key, field := range fields {
				if err, ok := valueErrorField(value, key); ok && key == "error" && eventError == nil {
					eventError = err
					continue
				}
				data[key] = field
			}
		} else {
			data["value"] = sanitized
		}
	}
	if eventError != nil {
		delete(data, "error")
	}
	return data, eventError
}

func valueErrorField(value any, key string) (error, bool) {
	if key != "error" {
		return nil, false
	}
	switch fields := value.(type) {
	case map[string]any:
		err, ok := fields[key].(error)
		return err, ok
	case types.LogData:
		err, ok := fields[key].(error)
		return err, ok
	default:
		return nil, false
	}
}

func severity(level types.LogLevel) int {
	switch level {
	case types.Debug:
		return 0
	case types.Info:
		return 1
	case types.Warn:
		return 2
	case types.Error:
		return 3
	case types.Fatal:
		return 4
	default:
		return 5
	}
}

func cloneConfig(cfg types.VLoggoConfig) types.VLoggoConfig {
	cfg.SMTP.To = append([]string(nil), cfg.SMTP.To...)
	cfg.Redact = append([]string(nil), cfg.Redact...)
	return cfg
}

func cloneData(data types.LogData) types.LogData {
	result := make(types.LogData, len(data))
	for key, value := range data {
		result[key] = value
	}
	return result
}

func rebaseDefaultDirectories(cfg *types.VLoggoConfig, previousClient string) {
	if cfg == nil || cfg.Client == previousClient {
		return
	}
	previous := config.DefaultDirectory(previousClient)
	next := config.DefaultDirectory(cfg.Client)
	if !cfg.DirectoryTxtSet && cfg.Directory.Txt == previous.Txt {
		cfg.Directory.Txt = next.Txt
	}
	if !cfg.DirectoryJsonSet && cfg.Directory.Json == previous.Json {
		cfg.Directory.Json = next.Json
	}
}
