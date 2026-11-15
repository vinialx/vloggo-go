# Migrating to VLoggo Go 2

VLoggo Go 2 keeps existing two-argument logging calls and legacy configuration options where practical. It adds independent outputs, severity filtering, structured event data, child context, redaction, rotation controls, SMTP settings, and lifecycle methods. Review these API and behavior changes before upgrading.

## Module path

Go 2 uses the `/v2` module path. Update the dependency and every import:

```sh
go get github.com/vinialx/vloggo-go/v2@v2.0.0
```

```go
import (
	vloggo "github.com/vinialx/vloggo-go/v2"
	"github.com/vinialx/vloggo-go/v2/config"
	"github.com/vinialx/vloggo-go/v2/types"
)
```

## Logging calls

Existing calls remain valid:

```go
logger.Info("APP_READY", "Service started")
logger.Error("DB_FAILED", "Database query failed")
```

Add structured metadata as variadic arguments. Prefer `types.LogData` for event fields and pass native Go errors as `error` values:

```go
logger.Error("DB_FAILED", "Database query failed",
	types.LogData{"database": "primary", "requestId": "req-8f21"},
	err,
)
```

Go severity constants are uppercase: `types.Debug`, `types.Info`, `types.Warn`, `types.Error`, and `types.Fatal`. String levels are normalized to uppercase during validation.

## Configuration API

Go 2 uses functional options instead of JavaScript object configuration:

```go
logger, err := vloggo.NewInstanceE("api",
	config.WithLevel(types.Warn),
	config.WithOutputs(types.VLoggoOutputs{
		Console: true,
		Text:    true,
		JSON:    true,
	}),
)
```

Legacy `WithConsole`, `WithJSON`, and `WithDebug` options remain supported. `WithDebug(true)` selects DEBUG only when no explicit level is set. The default level is INFO. Text, JSONL, and console outputs are independent; console-only mode does not create file directories.

Use `NewInstanceE`, `NewInstanceFromConfigE`, or `NewInstanceFromInputE` when the caller must handle validation errors. Compatibility constructors without the `E` suffix report invalid configuration through a diagnostic and return a default logger.

Remote JSON should decode into `types.VLoggoConfigInput`, whose pointer fields distinguish omitted settings from explicit `false` and `0`. Nested input objects merge with defaults. Use `GetConfig` for an independent snapshot and `Update` for validated changes. Invalid updates leave active settings unchanged.

## Defaults and output changes

- Default client comes from `CLIENT_NAME`, then `VLoggo`.
- Default directories are `~/[client]/logs` and `~/[client]/json`.
- Text timestamps use milliseconds and configured IANA timezone. JSONL timestamps remain UTC.
- Text IDs use `evt-` plus eight UUID hex characters. JSONL and email use full UUIDs.
- JSONL includes `eventId`, `client`, `level`, `timestamp`, `code`, `caller`, `message`, and optional `context`, `data`, and `error` fields.
- No startup banner is written unless `WithStartupMessage(true)` is set.
- Rotation supports daily files, size-based segments, retention days, file counts, and a combined storage limit. Size limits use byte counts.

Consumers that parse exact text field layouts or require the old JSON field set must update their parsers.

## Child loggers and redaction

Use `Child(types.LogData)` to attach request or job context. Child context is copied and nested children override inherited keys. Children share parent outputs, SMTP state, rotation, and lifecycle.

Redaction patterns match field names at any depth or dotted paths, including `*` wildcard segments. Redaction runs before output and does not mutate input values. Go errors do not have a standard stack interface; VLoggo includes stack text only when an error implements `StackTrace() string`.

## SMTP and fatal behavior

Notifications use `WithNotify`, `WithSMTP`, `WithThrottle`, `WithNotificationTimeout`, and `WithRetryDelay`. Environment values are read from `CLIENT_NAME` and `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`, and `SMTP_TO`. VLoggo does not load `.env` files.

When notifications are disabled, SMTP environment values are not loaded. Complete valid SMTP settings enable notifications when `WithNotify` is omitted. Partial environment settings remain silent. Explicit `WithNotify(true)` with invalid SMTP settings emits one diagnostic when console output is enabled and continues logging.

`Fatal` logs and waits synchronously for an enabled notification. It never calls `os.Exit`. Use `FatalContext` to apply a context deadline and receive notification errors. Unlike the JavaScript API, Go logging and SMTP delivery are synchronous.

## Shutdown

Call `Flush(ctx)` to wait for accepted events and cleanup. Call `Close(ctx)` to reject new events, drain active work, and release resources. `Close` is idempotent. VLoggo does not install signal handlers; applications should manage OS signals and shutdown contexts.
