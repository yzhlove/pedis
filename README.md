# pedis

> 一个基于加密 Unix 域套接字的 Redis 多路复用代理。

`pedis` 把"暴露 Redis 端口"和"连接真实 Redis"拆成两个进程：暴露端通过 TCP 向外提供 Redis 协议接口，连接端在内部网络访问真实 Redis；两端之间通过本机/挂载共享的 Unix 套接字通信，链路使用 X25519 密钥交换 + AES-256-GCM 加密。多个 client 通过自己的 13 字符 name（即客户端连接时使用的 AUTH 口令）在同一个 server 上复用，server 按照 name 路由到对应的 client。

## 应用场景

- 在容器/主机边界上拆分"对外暴露"和"实际访问 Redis"的职责，同一个 server 实例承载多个 client。
- 借助 AUTH 口令 = name 的设计，第三方 Redis 客户端无须改造，使用任意支持 `AUTH` 或 `HELLO ... AUTH ...` 的工具即可接入。
- Unix 套接字上的链路即使被旁观也无法识别命令内容（经过 AES-GCM 加密 + 协议帧封装）。

## 架构概览

```
                ┌────────────────────────────────────────────────┐
                │                     Host A                     │
                │                                                │
   redis-cli/   │   ┌──────────────┐         ┌──────────────┐   │
   tinydb/  ───►:6399│ pedis server │────────►│ pedis client │───┼────► 127.0.0.1:6379
   redisinsight│   │  (TCP 6399)  │  Unix    │              │   │       (real Redis)
                │   └──────────────┘  Socket └──────────────┘   │
                │       ▲                        ▲              │
                │       │ AUTH <name>            │ IKE handshake│
                │       │ HELLO 3 AUTH _ <name>  │ + AES-GCM    │
                │                                               │
                └───────────────────────────────────────────────┘
```

数据走向：

1. `pedis client` 进程主动连接 `pedis server` 的 Unix 套接字，完成 X25519 握手协商出会话密钥，并把自己注册到 server 的连接池（key 为 `cli_name`）。
2. 第三方 Redis 客户端连接 `pedis server` 的 TCP 端口（默认 6399），首条命令必须是 `AUTH <name>` 或带 AUTH 三元组的 `HELLO`。
3. `pedis server` 用 name 在注册表里查到一条空闲的 client 连接，把客户端 TCP 连接和 client 的 Unix 套接字桥接起来。AUTH 被代理拦截不下传、HELLO 中的 AUTH 三元组会被剥离后再转给真实 Redis。
4. 之后所有命令在两端之间双向透传。

## 项目结构

```
pedis/
  main.go                    入口，dig 装配 service/module
  config.json                默认配置
  Makefile / Dockerfile      构建与容器编排
  proto/
    msg.proto                Auth/String/Nil（握手与心跳消息）
    pb/                      生成的 Go 代码
  internal/
    config/                  Config 结构、读写超时常量
    cipher/                  X25519 身份 + AES-256-GCM 会话
    codec/                   Unix 套接字上的握手 / 心跳编解码
    conn/                    Connector 接口（redis/unix 两种实现）
    helper/                  桥接（io.Copy 双向）+ 缓冲池
    log/                     slog 包装
    module/                  Module.Apply() 顺序初始化
    packet/                  2 字节长度前缀帧
    redis/                   PING/OK/PONG 等 RESP 标准回包
    resp/                    RESP 协议 5 种类型 + 池化解析
    service/
      client/                客户端状态机：worker/manager/bridge
      server/                服务端：registry / redis_server / unix_server
    text/                    uint64 ↔ 13 字符 name 的编码
```

## 快速开始

### 1. 编译

```bash
# 本机原生构建
make build-local

# 跨平台构建并打 Docker 镜像
make build
```

或直接用 go：

```bash
go build -o pedis .
```

### 2. 配置

`config.json` 示例：

```json
{
  "server_public_key": "A4clrfenHKdzk2nC18phFzUbqeO3K4lTwUAHtn9wVBg=",
  "server_private_key": "TaB4DSNF8oUdwCW+Kdw/ZFduir+11AKEKmnZC71pK7o=",
  "time_seed": "psMrfhLXrUxbdyYdzzaAjQLr8mDwuu0c",
  "character_set": "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz@#$%*!+=/?"
}
```

所有字段都可以用环境变量覆盖（前缀 `PEDIS_`），常用配置：

| 字段 | 环境变量 | 说明 |
|------|----------|------|
| `role` | `PEDIS_ROLE` | `client` 或 `server`，决定本进程的角色 |
| `cli_name` | `PEDIS_CLI_NAME` | 仅 client 使用，本节点注册到 server 的 13 字符 name |
| `cli_redis_host` | `PEDIS_CLI_REDIS_HOST` | 真实 Redis 主机 |
| `cli_redis_port` | `PEDIS_CLI_REDIS_PORT` | 真实 Redis 端口 |
| `unix_socket` | `PEDIS_UNIX_SOCKET` | 两端共享的 Unix 套接字路径 |
| `server_port` | `PEDIS_SERVER_PORT` | server 对外暴露的 TCP 端口 |
| `server_public_key` | `PEDIS_SERVER_PUBLIC_KEY` | base32 X25519 公钥 |
| `server_private_key` | `PEDIS_SERVER_PRIVATE_KEY` | base32 X25519 私钥 |
| `time_seed` | `PEDIS_TIME_SEED` | text 包 name 编码的时间种子 |
| `character_set` | `PEDIS_CHARACTER_SET` | text 包字符表（影响 name 的字符空间） |

### 3. 启动

需要分别启动 server 和 client 两个进程，通过 `PEDIS_ROLE` 区分。

**Server（暴露 TCP 6399）：**

```bash
PEDIS_ROLE=server \
PEDIS_SERVER_PORT=6399 \
PEDIS_UNIX_SOCKET=/tmp/pedis.sock \
./pedis
```

**Client（连真实 Redis 127.0.0.1:6379，注册名 `yurisa`）：**

```bash
PEDIS_ROLE=client \
PEDIS_CLI_NAME=yurisa \
PEDIS_CLI_REDIS_HOST=127.0.0.1 \
PEDIS_CLI_REDIS_PORT=6379 \
PEDIS_UNIX_SOCKET=/tmp/pedis.sock \
./pedis
```

### 4. 用任意 Redis 客户端接入

```bash
# 以 name 作为 AUTH 口令
redis-cli -p 6399 -a yurisa
redis-cli -p 6399 AUTH yurisa
```

RESP3 客户端（TinyDB、RedisInsight 等）会发出 `HELLO 3 AUTH default <name>`，pedis server 会自动剥离 AUTH 三元组、把 `HELLO 3 [SETNAME ...]` 转给真实 Redis，让客户端拿到合法的 HELLO Map 响应。

### 5. Docker 部署

```bash
make network          # 创建 docker bridge 网络
make build            # 构建镜像
make server           # 启动 server 容器
make client           # 启动 client 容器
make stop             # 关闭
```

`server` 与 `client` 通过共享 `/tmp` 卷交换 Unix 套接字。

## 关键实现要点

### 桥接路径上的 RESP 过滤

`internal/service/server/filtered_conn.go` 实现了一个 `filteredConn`：包裹后端 Unix 连接，`Write` 通过 `io.Pipe` 把字节流喂给一个 goroutine，goroutine 用 `bufio.Reader + resp.GetObject` 按 RESP 协议解码出完整命令再决策：

- `AUTH ...` —— 拦截，直接给客户端回 `+OK`，不下传到真实 Redis（避免真实 Redis 没设密码时报 `ERR Client sent AUTH, but no password is set`）。
- `HELLO ... AUTH ... ...` —— 剥离 AUTH 三元组保留 `protover` / `SETNAME`，再转给真实 Redis。
- 其他命令 —— 重新编码后透传。

`Read` 直接走嵌入的 `net.Conn`，因为 backend → client 方向不需要任何过滤。

### 连接池

`registry.go` 用 `map[string][]*connEntry` 维护每个 name 的连接池，每个 entry 由独立 goroutine 负责心跳。`Get` 在桶里 LIFO 扫描，通过非阻塞 try-send 选一个可用 entry 移交；这样多个客户端同时连接（如 TinyDB 默认开 2 条）不会都撞同一个 entry。

### 握手 + 心跳

- Unix 套接字握手用 `proto/msg.proto` 中的 `Auth` 消息：客户端发 `Auth(timestamp, salt, dh_pub_key_bytes, signature, ecdsa_pub_key_bytes)`，服务端验签并返回自己的 ECDH 公钥；之后双方用 X25519 协商出 32 字节会话密钥，进入 AES-256-GCM 加密通道。
- TCP 侧（server ↔ 真实 Redis）通过 `PING / +PONG` 做空闲心跳，间隔由 `config.HeartbeatInterval`（2 分钟）控制。

### 模块初始化顺序

`module.Module` 接口只有一个 `Apply() error`。`text`（name 编码本）和 `cipher`（身份密钥）是有状态单例，在 `main.go` 通过 `module.Apply(...)` 按依赖顺序初始化后再启动 service。

## 开发常用命令

```bash
make test       # 跑全部测试
make lint       # golangci-lint
make proto      # 改动 proto/msg.proto 后重新生成
make help       # 列出所有 make 目标
```

或直接用 go：

```bash
go test ./...
go test ./internal/resp/...     # 单包测试
go vet ./...
```

## 依赖

- [`github.com/bytedance/gopkg`](https://github.com/bytedance/gopkg) — `mcache` 缓冲池
- [`github.com/jinzhu/configor`](https://github.com/jinzhu/configor) — JSON / 环境变量配置
- [`go.uber.org/dig`](https://pkg.go.dev/go.uber.org/dig) — 依赖注入
- [`google.golang.org/protobuf`](https://pkg.go.dev/google.golang.org/protobuf) — 握手 / 心跳消息
