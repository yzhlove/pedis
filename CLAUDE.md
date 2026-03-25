# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pedis** is a Redis tool written in Go. It implements a Redis client/server using the RESP (Redis Serialization Protocol) over raw TCP connections.

## Commands

```bash
# Build
go build ./...

# Run
go run main.go

# Test
go test ./...

# Test a single package
go test ./app/internal/parse/...

# Lint (if golangci-lint is available)
golangci-lint run
```

## Architecture

The project is in early development. Key packages:

- **`app/internal/medis`** — Core Redis connection manager. `RedisManager` interface wraps a raw TCP `net.Conn` with `Connect`, `Heartbeat`, `TcpConn`, and `Close` methods. `medis.New(host, port)` is the constructor.

- **`app/internal/parse`** — RESP protocol types. Implements the 5 RESP data types:
  - `Status` → `+OK\r\n`
  - `Error` → `-ERR ...\r\n`
  - `Integer` → `:42\r\n`
  - `Bulk` → bulk strings
  - `ArrBulk` → arrays
  - `Sep = "\r\n"` is the RESP line terminator defined in `common.go`.

- **`app/helper`** — Buffer pool helpers using `bytedance/gopkg/lang/mcache`. Always use `Get1KBBytes()` / `FreeBytes()` for temporary byte buffers (not `make([]byte, ...)`).

- **`app/service/server`** and **`app/service/client`** — Stubs for the server and client implementations (not yet implemented).

- **`app/config`** — Config structs (not yet populated).

## Module

The Go module is `github.com/yzhlove/peids` (note: module path uses "peids", not "pedis"). Import paths use this module name.

## Key Dependency

- `github.com/bytedance/gopkg` — used for `mcache` memory cache/pool to reduce GC pressure on temporary buffers.
