# Server Implementation Plan

## 目标

在 `internal/service/server/` 目录中实现服务器逻辑，管理两个子服务：
- **Redis Server**：监听 TCP 端口，接受第三方 Redis 客户端（redis-cli、medis 等）
- **Unix Server**：监听 Unix socket，接受 pedis unix client 连接

---

## 整体数据流

```
redis-cli ←─TCP──→ [Redis Server]
                         ↕ bridge (io.Copy 双向)
                   [Unix Server]
                         ↕ unix socket (raw)
                   [Unix Client (pedis client mode)]
                         ↕ bridge (io.Copy 双向)
                   [Local Real Redis]
```

---

## 文件结构

```
internal/service/server/
  server.go        — service.Service 入口（Init/Start/Stop）
  registry.go      — 线程安全的 name → unixEntry 注册表
  unix_server.go   — 监听 Unix socket，处理 IKE 握手，注册到 registry
  redis_server.go  — 监听 TCP，解析 RESP2 AUTH，从 registry 取 conn 做 bridge
```

同时需要修改：
- `cmd/pedis/main.go`：向 DI 容器注册 `server.New`

---

## 各组件设计

### 1. `server.go`

实现 `service.Service` 接口（`Init / Start / Stop`）。

- `Init()`：仅在 `cfg.Role == config.ServerRole` 时初始化，创建 registry、unix_server、redis_server
- `Start()`：同时启动 unix_server 和 redis_server 两个 goroutine
- `Stop()`：关闭监听 listener，取消 context

---

### 2. `registry.go`

```go
type unixEntry struct {
    conn    net.Conn           // 完成 IKE + Hello 后的原始连接
    sc      codec.ServerCodec  // 仍在 S2 session，用于响应 Heartbeat
    cancel  context.CancelFunc // 用于停止 heartbeat goroutine
    doneCh  chan struct{}       // heartbeat goroutine 退出信号
    taken   bool               // 已被 Redis client 取走
    mu      sync.Mutex
}

type Registry struct {
    mu      sync.RWMutex
    entries map[string]*unixEntry
}
```

主要方法：
- `Register(name string, e *unixEntry)` — 注册；若同名已存在则关闭旧连接，注册新的
- `Take(name string) net.Conn` — 从注册表取出连接（标记为 taken，停止 heartbeat goroutine，等待 goroutine 退出，清除 deadline，返回裸 conn）
- `Remove(name string)` — 连接断开时移除

---

### 3. `unix_server.go`

**连接生命周期：**

```
Accept → codec.NewServer() → Handle(Auth) → Handle(Hello) → Register in registry
       → goroutine: Heartbeat loop → Handle(Free) → rawCh <- conn
```

**核心逻辑：**

1. `net.Listen("unix", cfg.UnixSocket)` 开始监听
2. 每个新连接启动一个 goroutine：
   a. 创建 `codec.NewServer()` 得到 `ServerCodec`（即 `serverCodec`，同时实现 `ServerHandler`）
   b. 调用 `codec.Handle()` 处理 Auth（IKE 握手）
   c. 调用 `codec.Handle()` 处理 Hello，从 `sc.GetClientName()` 取得 client name
   d. 将 `(conn, sc)` 注册到 registry
   e. 启动 heartbeat 响应 goroutine（见下方）

**Heartbeat goroutine：**

```go
func (e *unixEntry) runHeartbeat(ctx context.Context) {
    defer close(e.doneCh)
    for {
        // 设置短 deadline，让 blocking read 定期检查 ctx
        e.conn.SetDeadline(time.Now().Add(1 * time.Second))
        free, err := codec.Handle(e.sc, e.conn)
        if ctx.Err() != nil {
            return // 被 Take() 或 context 取消
        }
        if isTimeout(err) {
            continue
        }
        if err != nil {
            // 连接断开，从 registry 移除
            registry.Remove(name)
            return
        }
        if free {
            // unix client 主动发送 Free（detach），conn 进入 raw 模式
            // 通知 registry：此 entry 已就绪，可以直接 bridge
            // 更新 entry 状态为 freeReady
            return
        }
    }
}
```

**Take() 流程：**

当 Redis Server 调用 `registry.Take(name)` 时：
1. 标记 entry 为 taken，防止竞态
2. 调用 `entry.cancel()`，取消 heartbeat goroutine 的 context
3. 设置 `entry.conn.SetDeadline(time.Now())` 立即打断阻塞的 Handle() 读操作
4. 等待 `<-entry.doneCh`，确保 goroutine 完全退出
5. 清除 deadline：`entry.conn.SetDeadline(time.Time{})`
6. 返回裸 `entry.conn`

---

### 4. `redis_server.go`

**连接生命周期：**

```
Accept → RESP2 command loop → AUTH → lookup registry → bridge → close
```

**核心逻辑：**

1. `net.Listen("tcp", ":" + cfg.ServerPort)` 开始监听
2. 每个新连接启动一个 goroutine：
   a. 循环读取 RESP2 命令（使用 `resp.GetObject()`）
   b. 解析命令数组（ArrBulk）：
      - 命令为 `AUTH passwd`（2 参数）：client name = `passwd`
      - 命令为 `AUTH username passwd`（3 参数）：client name = `username`（兼容新形式）
      - 其他命令（AUTH 之前）：返回 `-ERR NOAUTH Authentication required\r\n`
   c. AUTH 验证：`registry.Take(name)` 取出 unix conn
      - 取不到（name 不存在或未就绪）：返回 `-ERR no unix client connected for 'name'\r\n`，关闭连接
      - 取到：返回 `+OK\r\n`，启动 bridge
   d. Bridge：双向 `io.Copy(unixConn, redisConn)` + `io.Copy(redisConn, unixConn)`
   e. 任意一侧断开 → 两端均关闭（满足需求 4）

---

### 5. `cmd/pedis/main.go` 修改

在 DI 容器中添加：
```go
container.Provide(server.New, dig.Group("services"))
```

---

## 状态流转图

### Unix Client 连接状态（Server 视角）

```
Connected
    ↓ (Auth + Hello)
Registered (heartbeat goroutine running)
    ↓ (Take() or unix client sends Free)
Raw (heartbeat goroutine stopped, conn available for bridge)
    ↓ (bridge started)
Bridging
    ↓ (either side disconnects)
Closed
```

---

## 配置依赖

| Config 字段         | Server 用途                         |
|---------------------|-------------------------------------|
| `ServerPrivateKey`  | 服务器 X25519 私钥（cipher 模块）     |
| `UnixSocket`        | Unix socket 监听路径（服务端 listen）  |
| `ServerPort`        | Redis Server TCP 监听端口             |
| `Salt`              | cipher 模块 HKDF salt（32 字节）      |

---

## 需要与你确认的问题（存疑点）

---

### Q1：unix client 发送 Free 的时机 ⭐ 最关键

**背景：**
unix client 当前的代码逻辑是：
- 只有当 unix 连接 **和** 本地 Redis 连接**都就绪**时，manager 才发 `CmdDetachForBridge`，让 unix worker 发送 `Free`。
- 即 unix client 只有本地 Redis 也 OK 之后才发 Free，才进入 raw bridge 模式。

**问题：**
如果第三方 Redis 客户端先连上 Redis Server 并 AUTH 了，但此时 unix client 的本地 Redis 还没连上（或 unix client 还在 heartbeat 阶段），registry 里还没有 raw conn 可用。

**有三种处理方式，请确认哪种符合预期：**

- **方案 A（推荐，无需改客户端）**：Redis Server 在 AUTH 成功后，如果 registry 里 entry 存在但还没进入 raw 模式（Free 未到），就**等待**，直到 unix client 发来 Free（raw conn 就绪后）再 bridge。等待超时（如 30s）则断开 Redis 客户端。
- **方案 B（立即报错）**：如果 unix client 还未发送 Free，直接返回错误给 Redis 客户端（`-ERR client not ready`），Redis 客户端需要重试。
- **方案 C（需要修改 codec + client）**：server 在 registry 中查到 entry 后，主动发送一个新协议命令（如 `BridgeCmd`）给 unix client，要求其立即发送 Free。这需要改 codec 和 client 代码。

---

### Q2：AUTH 之前，Redis 客户端发送其他命令如何处理？

很多 Redis 客户端在发 AUTH 前会发其他命令：
- `HELLO 2` / `HELLO 3`（RESP3 协商）
- `CLIENT SETNAME foo`
- `PING`

**建议方案（请确认）：**
- `PING` → 返回 `+PONG\r\n`
- `HELLO 2` → 返回 `-ERR RESP2 only supported\r\n`（让客户端回退到 RESP2）
- 其他命令（AUTH 前）→ 返回 `-ERR NOAUTH Authentication required\r\n`

---

### Q3：同名 unix client 重复连接

如果 client name 为 `foo` 的 unix client 已经注册，又来了一个同名新连接：

- **方案 A（推荐）**：关闭旧连接，注册新的（last-one-wins）
- **方案 B**：拒绝新连接（直接关掉）

---

### Q4：Redis client AUTH 时，没有对应名字的 unix client

- **方案 A（推荐）**：立即返回 `-ERR no client 'name' connected\r\n`，断开连接
- **方案 B**：等待一段时间（如 10s），如果期间有 unix client 以该名字注册则 bridge，超时则报错

---

### Q5：`cmd/pedis/main.go` 中，server.New 和 client.New 是否应该根据 Role 条件注册？

目前 client.New 已经在内部通过 `isRunning()` 判断 `cfg.Role == ClientRole` 来决定是否启动。

Server 是否也用同样的方式（service 内部判断 role）？还是在 main.go 中根据 role 条件性地注册 provider？

**建议（请确认）**：沿用 client 的做法，在 `server.Service` 内部通过 `cfg.Role == ServerRole` 来决定是否真正启动。

---

## 总结

核心不确定的是 **Q1（unix client 发 Free 的时机）**，其他问题相对有明确的推荐方案。请就 Q1～Q5 与我确认后，即可开始编码。
