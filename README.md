# VLoggo Go

Logging library for Go with daily file rotation, configurable retention, JSONL output, caller tracking, console output and fatal SMTP notifications.

## Features

- Levels: `INFO`, `WARN`, `DEBUG`, `ERROR`, `FATAL`
- Daily log files: `log-YYYY-MM-DD.txt` and optional `log-YYYY-MM-DD.jsonl`
- Retention by day/file count: keeps newest `Filecount.Txt` and `Filecount.Json`, deletes older logs
- Configurable through constructor options or remote JSON input
- No `.env` dependency
- Optional console output
- Optional fatal email notification with throttle
- Caller file/line tracking

## Nix Development

```sh
direnv allow
go test ./...
```

Without direnv:

```sh
nix develop
go test ./...
```

## Usage

```go
package main

import (
    vloggo "github.com/vinialx/vloggo-go"
    "github.com/vinialx/vloggo-go/config"
    "github.com/vinialx/vloggo-go/types"
)

func main() {
    logger := vloggo.NewInstance("api",
        config.WithJSON(true),
        config.WithConsole(true),
        config.WithFilecount(types.Count{Txt: 7, Json: 7}),
        config.WithDirectory(types.Paths{
            Txt:  "/var/log/api",
            Json: "/var/log/api-json",
        }),
    )

    logger.Info("BOOT", "service started")
}
```

## Remote Config

Use pointer fields so omitted remote values keep defaults, while explicit `false` and `0` can still be represented.

```go
client := "api"
jsonEnabled := true
retention := 14

logger := vloggo.NewInstanceFromInput(types.VLoggoConfigInput{
    Client: &client,
    Json:   &jsonEnabled,
    Filecount: &types.CountInput{
        Txt:  &retention,
        Json: &retention,
    },
})
```

## Fatal Email

Email is only sent by `Fatal`, and it is throttled by `Throttle` in milliseconds.

```go
logger := vloggo.NewInstance("api",
    config.WithNotify(true),
    config.WithThrottle(30000),
    config.WithSMTP(types.VLoggoSMTP{
        Host:     "smtp.example.com",
        Port:     587,
        Username: "user",
        Password: "pass",
        From:     "logs@example.com",
        To:       []string{"dev@example.com"},
        Secure:   false,
    }),
)
```
