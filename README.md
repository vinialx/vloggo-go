# VLoggo Go

VLoggo Go is a structured logging library with independent console, text, and JSONL outputs. It supports severity filtering, child context, redaction, timezone formatting, file rotation, retention, fatal SMTP notifications, and explicit lifecycle control.

## Requirements

- Go 1.24.3 or newer.
- No third-party Go modules.

## Installation

```sh
go get github.com/vinialx/vloggo-go/v2
```

## Quick start

```go
package main

import (
	"context"

	vloggo "github.com/vinialx/vloggo-go/v2"
	"github.com/vinialx/vloggo-go/v2/config"
	"github.com/vinialx/vloggo-go/v2/types"
)

func main() {
	logger, err := vloggo.NewInstanceE("api",
		config.WithOutputs(types.VLoggoOutputs{
			Console: true,
			Text:    true,
			JSON:    true,
		}),
	)
	if err != nil {
		panic(err)
	}
	defer logger.Close(context.Background())

	logger.Info("APP_READY", "Service started", types.LogData{"port": 3000})
	logger.Warn("HIGH_MEMORY", "Memory use is high", types.LogData{"percent": 82})
	logger.Error("API_FAIL", "External service is unavailable", types.LogData{"service": "billing"})
}
```

Console and text output use one line per event:

```text
[api] [12/11/2026 14:32:08.125] [evt-550e8400] [INFO ] [APP_READY] : Service started > port=3000
```

Text IDs contain `evt-` and first eight hexadecimal UUID characters. JSONL and email include full UUID. VLoggo does not emit startup banners, separators, or SMTP messages by default.

## Configuration

Options use Go functional options. `NewInstanceE` returns validation errors. `NewInstance` keeps a compatibility-friendly signature and emits a configuration diagnostic when options are invalid.

```go
logger, err := vloggo.NewInstanceE("api",
	config.WithLevel(types.Warn),
	config.WithOutputs(types.VLoggoOutputs{
		Console: true,
		Text:    true,
		JSON:    true,
	}),
	config.WithDirectory(types.Paths{
		Txt:  "/var/log/api/text",
		Json: "/var/log/api/json",
	}),
	config.WithFormat(types.VLoggoFormat{Timezone: "America/Sao_Paulo"}),
	config.WithRotation(types.VLoggoRotation{
		RetentionDays: 14,
		MaxFileSize:   20 * 1024 * 1024,
		MaxTotalSize:  1024 * 1024 * 1024,
	}),
	config.WithRedact("password", "headers.authorization"),
)
```

### Defaults

| Option | Default | Meaning |
| --- | --- | --- |
| `client` | `CLIENT_NAME` or `VLoggo` | Application name and default directory name |
| `level` | `INFO` | Lowest severity written |
| `outputs.console` | `true` | Print events to stdout or stderr |
| `outputs.text` | `true` | Write one-line text events |
| `outputs.json` | `false` | Write JSON Lines events |
| `directory.txt` | `~/[client]/logs` | Text output directory |
| `directory.json` | `~/[client]/json` | JSONL output directory |
| `filecount.txt`, `filecount.json` | `31` | Maximum retained files per output |
| `rotation.retentionDays` | `31` | Maximum age of managed daily files |
| `rotation.maxFileSize` | `20 MiB` | Start a new segment before an append exceeds this byte count |
| `rotation.maxTotalSize` | `1 GiB` | Maximum managed file size across enabled outputs |
| `format.timezone` | System timezone | Text timestamps and daily filenames |
| `redact` | Empty | Field names or paths to replace with `[REDACTED]` |
| `startupMessage` | `false` | Emit one `APP_START` event during construction |
| `notify` | `true` only with valid SMTP settings | Send email for fatal events |
| `throttle` | `30000` ms | Minimum time between successful notifications |
| `notificationTimeout` | `10000` ms | Maximum duration of one notification attempt |
| `retryDelay` | `1000` ms | Delay before retry after a failed notification |

Rotation sizes are positive `int64` byte counts. Zero fields in `VLoggoRotation` select defaults; remote input uses pointers and rejects explicit invalid zero values. Outputs are independent. Set `Text` and `JSON` to false for console-only logging; VLoggo then creates no log directories.

### Log levels and legacy aliases

Levels are `DEBUG`, `INFO`, `WARN`, `ERROR`, and `FATAL`, from least to most severe. `WithDebug(true)` selects `DEBUG` when no explicit level is set. An explicit level takes precedence.

Legacy options remain supported:

```go
logger := vloggo.NewInstance("api",
	config.WithConsole(true),
	config.WithJSON(true),
	config.WithDebug(true),
)
```

These map to `Outputs.Console`, `Outputs.JSON`, and `Level: DEBUG`. `WithOutputs` sets all destinations together.

### Remote configuration

Pointer fields distinguish omitted values from explicit `false` or `0`:

```go
client := "api"
jsonEnabled := true
retention := 14

logger, err := vloggo.NewInstanceFromInputE(types.VLoggoConfigInput{
	Client: &client,
	Outputs: &types.VLoggoOutputsInput{JSON: &jsonEnabled},
	Filecount: &types.CountInput{Txt: &retention, Json: &retention},
})
```

Partial nested inputs retain defaults for omitted fields. Invalid explicit values return errors from the `...E` constructors.

## Structured events, children, and redaction

Pass optional `types.LogData` metadata as the third argument:

```go
logger.Info("HTTP_START", "Request received", types.LogData{
	"method": "POST",
	"path":   "/login",
})

requestLogger := logger.Child(types.LogData{"requestId": "req-8f21", "userId": "usr-42"})
requestLogger.Info("DB_QUERY", "Loading account", types.LogData{"database": "primary"})
```

Children copy context when created and can be nested. More specific child values override inherited keys. Children share configuration, outputs, SMTP throttling, file rotation, and lifecycle with parent.

Pass native Go `error` values directly or under an `error` metadata key. VLoggo serializes the error name and message, wrapped causes, and exported fields. Go does not define a standard stack API; errors that implement `StackTrace() string` include that stack. Redaction applies before all outputs and does not mutate caller-owned maps or structs. Circular values become `[Circular]`; excessive nesting becomes `[MaxDepth]`.

```go
logger := vloggo.NewInstance("api", config.WithRedact(
	"password",
	"headers.authorization",
	"users.*.token",
))

logger.Info("LOGIN", "Credentials checked", types.LogData{
	"password": "hidden",
	"headers":  map[string]any{"authorization": "Bearer hidden"},
})
```

Field-name patterns match at any depth. Dotted paths match nested values; `*` matches one object key or array index. Redaction cannot detect secrets embedded in message text or arbitrary stack locations.

## Text and JSONL formats

Text and console timestamps use configured IANA timezone with millisecond precision. Daily filenames use the same timezone. JSON timestamps use ISO 8601 UTC. Each event has one full UUID shared by enabled outputs.

JSONL contains one JSON object per line:

```json
{"eventId":"550e8400-e29b-41d4-a716-446655440000","client":"api","level":"INFO","timestamp":"2026-11-12T17:32:09.041Z","code":"HTTP_START","caller":"routes.go:42","message":"Request received","data":{"method":"POST"}}
```

Optional `context`, `data`, and `error` fields are omitted when absent. Caller identifies source filename and line.

## Rotation and retention

- Daily rotation occurs on the next event after the configured local date changes; no midnight timer runs.
- Files use `log-YYYY-MM-DD.txt` and `log-YYYY-MM-DD.001.txt`; JSONL files use `.jsonl`.
- A single event larger than `maxFileSize` remains intact in its own segment.
- Cleanup runs at initialization and after writes. `retentionDays`, `filecount`, and `maxTotalSize` all apply.
- Active files are protected. Limits can be exceeded while protected files are active.
- Cleanup only considers regular files with VLoggo-managed names. It does not follow or remove symlinks.
- Appends and retention cleanup are synchronous. Separate processes using the same directories do not coordinate rotation or active-file protection.

Use dedicated log directories for each process.

## SMTP notifications

Only `Fatal` sends email. SMTP protocol debugging uses `VLoggoSMTP.Debug`, separate from application severity.

```go
logger, err := vloggo.NewInstanceE("api",
	config.WithNotify(true),
	config.WithThrottle(300_000),
	config.WithNotificationTimeout(10_000),
	config.WithRetryDelay(1_000),
	config.WithSMTP(types.VLoggoSMTP{
		Host:     "smtp.example.com",
		Port:     465,
		Username: "service-account",
		Password: "replace-with-a-secret",
		From:     "logs@example.com",
		To:       []string{"ops@example.com"},
	}),
)
if err != nil {
	panic(err)
}

if err := logger.FatalContext(context.Background(), "DB_UNAVAILABLE", "Database is unavailable", types.LogData{"requestId": "req-8f21"}); err != nil {
	// The event was logged. Notification attempt failed.
}
```

`Fatal` logs and waits for any enabled notification but never calls `os.Exit`. It does not return delivery errors; diagnostics go to stderr when console output is enabled. Use `FatalContext` to bound the attempt and receive its error. Email includes the full event UUID, escaped HTML, a plain-text alternative, and redacted event details.

When `notify` is false, VLoggo skips SMTP environment loading. If `notify` is omitted, only complete valid SMTP settings enable delivery. Partial environment configuration stays silent. Explicit `notify: true` with invalid settings emits one `SMTP_CONFIG` diagnostic when console output is enabled, then continues logging.

`Secure` defaults to true on port `465`. For an explicit `false` on that port, set `SecureSet: true`; pointer-based remote input preserves explicit values automatically.

Supported environment values: `CLIENT_NAME`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_TO`, and optional `SMTP_SECURE`. `SMTP_TO` accepts comma-separated addresses. VLoggo does not load `.env` files.

## Updates and lifecycle

`GetConfig` returns an independent snapshot. Apply changes with `Update`; invalid changes leave active settings unchanged:

```go
if err := logger.Update(
	config.WithLevel(types.Debug),
	config.WithThrottle(60_000),
	config.WithFormat(types.VLoggoFormat{Timezone: "UTC"}),
); err != nil {
	return err
}
```

Use `Flush` and `Close` during shutdown:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := logger.Flush(ctx); err != nil {
	return err
}
if err := logger.Close(ctx); err != nil {
	return err
}
```

`Flush` waits for accepted events and retention cleanup. `Close` rejects new events, waits for in-flight work, and releases active files. `Close` is idempotent. VLoggo does not install process signal handlers.

`NewInstance` returns the same logger for repeated calls with the same client. Use `NewInstanceFromConfigE` to create a logger from a full configuration, `NewInstanceFromInputE` for pointer-based input, and `Clone` to copy a registered logger's settings under a new client name. `RemoveInstance` and `ClearInstances` close removed loggers.

## Migration

See [MIGRATION.md](MIGRATION.md) for migration notes from older Go APIs and JavaScript VLoggo 2 configuration.

## License

ISC. See [LICENSE](LICENSE).
