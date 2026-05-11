# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pedis** is a Redis proxy tool written in Go. It connects to a local Redis instance via TCP and exposes it to clients over a Unix socket, with a custom encrypted tunnel in between (X25519 key exchange + AES-256-GCM).

## Commands

```bash
# Build
go build ./...

# Run
go run .

# Test
go test ./...

# Test a single package
go test ./internal/resp/...

# Regenerate protobuf
make proto

# Lint (if golangci-lint is available)
golangci-lint run
```

## Module

Go module: `github.com/yzhlove/pedis`

## Project Structure

```
pedis/
  main.go             — entry point (fx DI + lifecycle wiring)
  internal/
    config/           — Config struct, loaded from config.json or env vars
    cipher/           — AES-256-GCM session + X25519 identity (modules.Module)
    codec/            — Client/server framing codec (IKE auth + heartbeat)
    conn/             — Connector interface; redis.go and unix.go implementations
    helper/           — Buffer pool via bytedance/gopkg mcache
    log/              — Thin slog wrapper with source-trimming and level control
    module/           — Module interface + Apply() for ordered initialization
    packet/           — 2-byte length-prefixed framing (Pack / Unpack)
    redis/            — Redis standard commands (PING)
    resp/             — RESP protocol: 5 types (Status/Error/Integer/Bulk/ArrBulk)
    service/
      client/         — State-machine client: worker, manager, bridge
      server/         — Listeners + registry; registers fx.Lifecycle hooks
    text/             — uint64 ↔ 13-char string encoding (book cipher)
  proto/
    msg.proto         — Protobuf source (snake_case fields)
    pb/               — Generated Go code (package pb)
  config.json         — Default runtime config
  Makefile
```

## Key Packages

### `internal/resp`
RESP protocol types. Implements 5 data types:
- `Status` → `+OK\r\n`
- `Error` → `-ERR ...\r\n`
- `Integer` → `:42\r\n`
- `Bulk` → bulk strings
- `ArrBulk` → arrays

All types are pool-allocated. Always use `GetXxx()` / `FreeXxx()` pairs. `Sep = "\r\n"` is defined in `common.go`.

### `internal/codec`
Custom framing protocol over Unix sockets. `ClientCodec` / `ServerCodec` interfaces handle encode/decode. `Auth()` and `Heartbeat()` are the two top-level operations.

### `internal/conn`
`Connector` interface with two implementations:
- `NewRedis(cfg)` — connects to Redis via TCP, heartbeat via PING/PONG
- `NewUnix(cfg)` — connects via Unix socket, performs IKE auth on connect

### `internal/service/client`
Event-driven state machine managing two concurrent workers (unix + redis). States: `NoneUp` → `UnixUpOnly` / `RedisUpOnly` → `PreparingBridge` → `Bridging`. When both connections are ready, a transparent `io.Copy` bridge is established.

### `internal/cipher`
- `identity.go` — X25519 long-term key pair, loaded from config (base32-encoded). Exposes `GetPubKey()` / `GetPrivKey()` / `GenerateKey()`.
- `session.go` — AES-256-GCM `Session` with monotonic counter nonce.

### `internal/service/server`
Constructor `server.New(lc fx.Lifecycle, sh fx.Shutdowner, cfg *Config) error` registers `OnStart` / `OnStop` hooks for the unix and redis listeners. A serve-loop error triggers `sh.Shutdown(...)` so the whole fx app tears down cleanly (preserves the legacy "any service fails → all stop" semantic).

### `internal/service/client`
Constructor `client.New(lc fx.Lifecycle, cfg *Config) error` builds N managers and registers a single `OnStart` hook that fans them out into goroutines, plus an `OnStop` that cancels the shared context and waits for them to finish.

### `internal/text`
Encodes `uint64` to a 13-character obfuscated string using a seed-derived book cipher. `Encode(val)` / `Decode(s)` are the public API. Requires `New(cfg).Apply()` to be called first (done via fx in `main.go`, ordered before any service constructor runs).

### `internal/module`
Minimal `Module` interface with a single `Apply() error` method. Used for ordered initialization of stateful singletons (`cipher`, `text`). They are provided into the `group:"modules"` fx group and applied via a dedicated `fx.Invoke` before any service constructor runs.

### `internal/helper`
Buffer pool using `bytedance/gopkg/lang/mcache`. Always use `Get1KBBytes()` / `FreeBytes()` for temporary byte buffers instead of `make([]byte, ...)`.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/bytedance/gopkg` | `mcache` buffer pool to reduce GC pressure |
| `github.com/jinzhu/configor` | Config loading from JSON / env vars |
| `go.uber.org/fx` | DI container + application lifecycle (signal handling, ordered start/stop) |
| `google.golang.org/protobuf` | Protobuf serialization for auth/heartbeat messages |

## Conventions

- **No global mutable state except module singletons** — `cipher.defaultIdentity`, `text.defaultBook`, `log.logger` are set once during `Apply()` and read-only afterwards.
- **Error messages** — lowercase, no trailing punctuation (e.g. `errors.New("conn: dial failed")`).
- **Logging** — always use `internal/log` wrappers (`log.Info`, `log.Error`, etc.), never `fmt.Println` or `log/slog` directly.
- **Temporary buffers** — use `helper.Get1KBBytes()` / `helper.FreeBytes()`, not `make([]byte, ...)`.
- **Proto** — regenerate with `make proto` after editing `proto/msg.proto`. Do not hand-edit `proto/pb/msg.pb.go`.
