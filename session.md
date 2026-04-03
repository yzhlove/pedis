# 会话记录：密码学方案讨论

> 日期：2026-04-03
> 起点文件：`app/modules/text/book.go`

---

## 一、PCG 随机数发生器的适用性

### 背景

`book.go` 第 63 行使用 PCG（Permuted Congruential Generator）生成密码本：

```go
h := fnv.New64a()
h.Write([]byte(cfg.TimeSeed))
seed := h.Sum64()
rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafe1234))
```

使用场景：传入时间戳 → `Encode` 计算字符串 → 通过 HKDF 派生密钥 → 加密通信消息。

### 问题

| 问题 | 说明 |
|------|------|
| PCG 不是 CSPRNG | PCG 统计质量出色，但不具备密码学安全性，输出可预测 |
| 时间戳熵极低 | 时间戳通常精度有限，攻击者可穷举猜测 |
| 两者叠加 | 安全性趋近于零 |

**CSPRNG（Cryptographically Secure PRNG）** 的核心要求：给定任意长度的输出，无法在多项式时间内区分它与真随机流，也无法推断过去或未来的输出。PCG 不满足此要求。

### 更好的方案

```go
// 直接用 crypto/rand 生成 IKM，跳过 PCG
ikm := make([]byte, 32)
if _, err := io.ReadFull(rand.Reader, ikm); err != nil {
    panic(err)
}
```

或者重构架构，让 IKM 来自真正的密钥协商（见第二节），而不是时间戳。

---

## 二、X25519 → HKDF → AEAD 完整加密链路

### 整体流程

```
[Alice]                          [Bob]
  |                                |
  |  生成临时密钥对 (a, A=a·G)      |  生成临时密钥对 (b, B=b·G)
  |                                |
  |  -------- 发送 A_pub --------> |
  |  <------- 发送 B_pub --------- |
  |                                |
  |  shared = X25519(a, B)         |  shared = X25519(b, A)
  |                                |  （两侧计算结果相同）
  |                                |
  |  HKDF(IKM=shared, salt=rand, info="v1") → key (32 bytes)
  |                                |
  |  AES-256-GCM / ChaCha20-Poly1305 加密消息
```

### 第一步：X25519 密钥协商

```go
import "crypto/ecdh"

// 生成临时密钥对
priv, _ := ecdh.X25519().GenerateKey(rand.Reader)
pub := priv.PublicKey()

// 计算共享秘密（对方公钥 peerPub 已通过某种方式获得）
shared, _ := priv.ECDH(peerPub)
```

- X25519 基于 Curve25519，天然防时序攻击
- 私钥 32 字节，公钥 32 字节，共享秘密 32 字节
- 安全性基于 ECDLP（椭圆曲线离散对数问题）

### 第二步：HKDF 密钥派生（RFC 5869）

```go
import "golang.org/x/crypto/hkdf"

salt := make([]byte, 32)
io.ReadFull(rand.Reader, salt)  // 每次会话随机生成

reader := hkdf.New(sha256.New, shared, salt, []byte("pedis-v1"))
key := make([]byte, 32)
io.ReadFull(reader, key)
```

**为何不直接用 shared？**

| 原因 | 说明 |
|------|------|
| 长度固定化 | HKDF 可输出任意长度密钥 |
| 增加熵混合 | 加入 salt 和 info，使派生密钥更均匀 |
| 领域分离 | 不同用途（加密/MAC）派生不同密钥 |
| 防 reuse | 相同 shared + 不同 salt → 不同 key |

HKDF 分两阶段：
- **Extract**：`PRK = HMAC(salt, IKM)` — 将输入熵"提取"为均匀伪随机密钥
- **Expand**：`OKM = T(1) || T(2) || ...` — 将 PRK 扩展为所需长度

### 第三步：AEAD 加密

```go
import (
    "crypto/aes"
    "crypto/cipher"
)

block, _ := aes.NewCipher(key)
gcm, _ := cipher.NewGCM(block)

nonce := make([]byte, gcm.NonceSize())
// 推荐：用单调递增计数器而非随机数，避免 nonce 碰撞
binary.BigEndian.PutUint64(nonce[4:], msgCounter)

ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)
```

**AES-256-GCM vs ChaCha20-Poly1305：**

| 算法 | 适用场景 |
|------|---------|
| AES-256-GCM | 有 AES-NI 硬件加速（x86/ARM）时首选 |
| ChaCha20-Poly1305 | 无硬件加速（嵌入式/IoT）时首选 |

两者都是 AEAD（Authenticated Encryption with Associated Data），同时提供：
- **机密性**：加密内容
- **完整性**：防篡改
- **认证**：与附加数据绑定

---

## 三、密钥协商时避免明文传输

### 问题背景

X25519 协商时，公钥必须传输给对方。用户考虑的两种方案：

#### 方案 A：双方写死密钥

**问题：**
- 密钥一旦泄露，所有历史会话全部可解密
- 无前向保密（Forward Secrecy）
- 密钥轮换困难，需要重新部署

#### 方案 B：book.go + 时间戳

**问题：**
- 时间戳精度有限，攻击者可穷举
- PCG 不是 CSPRNG，生成的密码本可被重建
- 相当于明文传输时间戳（时间本身不是秘密）

### 核心认知纠正

**X25519 公钥明文传输本身不是安全问题。**

X25519 的设计就是公钥公开传输——即使攻击者截获双方公钥，也无法计算出共享秘密（ECDLP 困难性保证）。

真正的威胁是 **MITM（中间人攻击）**：
```
Alice → 发送 A_pub → [Mallory 替换为 M_pub] → Bob
Bob   → 发送 B_pub → [Mallory 替换为 M_pub] → Alice

结果：
  Alice 与 Mallory 建立 shared_AM
  Bob   与 Mallory 建立 shared_BM
  Mallory 在中间解密+转发，双方以为在直接通信
```

**解决方案：身份认证**

---

## 四、方式一：预分发长期公钥（WireGuard 模式）

### 核心思想

带外（Out-of-Band）配置双方长期公钥，握手时通过 DH 运算隐式认证身份。

### 密钥体系

```
长期密钥（带外配置，固定）：
  Server: 长期私钥 b_s，长期公钥 B_s = b_s·G
  Client: 长期私钥 a_s，长期公钥 A_s = a_s·G

临时密钥（每次连接生成，用完销毁）：
  Client: 临时私钥 a_e，临时公钥 A_e = a_e·G
  Server: 临时私钥 b_e，临时公钥 B_e = b_e·G
```

### 握手协议

**M1：Client → Server**
```
{
  A_e (临时公钥, 明文),
  A_s (长期公钥, 用 dh_es 加密),
  Payload (可选, 用 dh_es+dh_ss 加密)
}
```

**M2：Server → Client**
```
{
  B_e (临时公钥, 明文),
  Payload (可选, 用所有 DH 混合密钥加密)
}
```

### 四个 DH 运算

```
dh_ee = X25519(a_e, B_e)  // 临时-临时：提供前向保密
dh_es = X25519(a_e, B_s)  // 临时-长期：认证服务端身份
dh_se = X25519(a_s, B_e)  // 长期-临时：认证客户端身份
dh_ss = X25519(a_s, B_s)  // 长期-长期：绑定双方身份
```

**会话密钥派生：**
```
IKM = dh_ee || dh_es || dh_se || dh_ss
session_key = HKDF(IKM, salt, "pedis-session-v1")
```

### 为何 MITM 失败

Mallory 没有 `b_s`（服务端长期私钥），无法计算：
```
dh_es = X25519(a_e, B_s)  // 需要 b_s 才能验证
dh_ss = X25519(a_s, B_s)  // 同上
```

即使 Mallory 替换了临时公钥，只要 `B_s` 是带外预配置的真实公钥，混合 IKM 就无法伪造。

### 前向保密分析

| 泄露场景 | 影响 |
|---------|------|
| 临时私钥泄露 | 仅影响当次会话 |
| 长期私钥泄露 | 未来会话受影响，但已销毁的临时私钥无法重建历史 `dh_ee` |
| 录制历史流量 + 日后获得长期私钥 | 无法解密，因为 `a_e`/`b_e` 已销毁 |

---

## 五、临时密钥的实现

### 生命周期

临时密钥活在 goroutine 的函数栈上：
- 不存入全局变量
- 不写磁盘
- 函数返回后由 GC 回收
- 通过 `runtime.SetFinalizer` 可主动清零（可选）

### Go 实现示例

```go
package main

import (
    "crypto/ecdh"
    "crypto/rand"
    "fmt"
)

func clientHandshake(serverLongTermPub *ecdh.PublicKey) ([]byte, error) {
    curve := ecdh.X25519()

    // 生成临时密钥对（活在本函数栈上）
    ephemeralPriv, err := curve.GenerateKey(rand.Reader)
    if err != nil {
        return nil, err
    }
    ephemeralPub := ephemeralPriv.PublicKey()

    // 将 ephemeralPub.Bytes() 发送给服务端...
    _ = ephemeralPub.Bytes()

    // 从服务端接收到 serverEphemeralPub 后...
    // dh_ee = X25519(a_e, B_e)
    // dh_es = X25519(a_e, B_s)  ← 用带外获取的 serverLongTermPub
    dh_es, err := ephemeralPriv.ECDH(serverLongTermPub)
    if err != nil {
        return nil, err
    }

    // 函数返回后 ephemeralPriv 超出作用域，GC 自动回收
    return dh_es, nil
}

func serverHandshake(serverLongTermPriv *ecdh.PrivateKey, clientEphemeralPub *ecdh.PublicKey) ([]byte, error) {
    curve := ecdh.X25519()

    // 生成服务端临时密钥对
    ephemeralPriv, err := curve.GenerateKey(rand.Reader)
    if err != nil {
        return nil, err
    }
    ephemeralPub := ephemeralPriv.PublicKey()

    // 将 ephemeralPub.Bytes() 发送给客户端...
    _ = ephemeralPub.Bytes()

    // dh_ee = X25519(b_e, A_e)
    dh_ee, err := ephemeralPriv.ECDH(clientEphemeralPub)
    if err != nil {
        return nil, err
    }

    // dh_se = X25519(b_s, A_e)  ← 用长期私钥认证客户端
    dh_se, err := serverLongTermPriv.ECDH(clientEphemeralPub)
    if err != nil {
        return nil, err
    }

    fmt.Printf("dh_ee: %x\ndh_se: %x\n", dh_ee, dh_se)

    // 函数返回后 ephemeralPriv 超出作用域
    return dh_ee, nil
}

func main() {
    curve := ecdh.X25519()

    // 带外预配置：服务端长期密钥
    serverLongTermPriv, _ := curve.GenerateKey(rand.Reader)
    serverLongTermPub := serverLongTermPriv.PublicKey()

    // 客户端握手（实际中通过网络交换公钥）
    _, _ = clientHandshake(serverLongTermPub)
    _, _ = serverHandshake(serverLongTermPriv, serverLongTermPub) // 简化演示
}
```

### 关键点

```
临时密钥                    长期密钥
─────────────────────      ─────────────────────
每次连接重新生成             带外配置一次，长期保存
函数退出即销毁               存储在安全位置（文件/HSM）
提供前向保密                 提供身份认证
不需要存储                   需要保护
```

---

## 六、其他方案对比

| 方案 | 适用场景 | 优点 | 缺点 |
|------|---------|------|------|
| **预分发长期公钥（本文方案一）** | 服务端固定，客户端可配置 | 无 CA 依赖，轻量 | 需带外分发公钥 |
| **PAKE（SPAKE2/OPAQUE）** | 双方共享口令 | 无需 PKI | 口令管理复杂 |
| **mTLS** | 企业内网，有 CA | 标准化，工具链完善 | 需要 CA 基础设施 |
| **Noise 协议框架** | 自定义协议 | 灵活，WireGuard 在用 | 需要自行实现 |

---

## 七、pedis 项目建议

基于以上讨论，对 `pedis` 项目的密码学方案建议：

1. **废弃 PCG + 时间戳方案**：`book.go` 中的密码本可保留用于消息混淆，但不应作为密钥派生的熵源

2. **采用 X25519 + HKDF + ChaCha20-Poly1305**：
   - `crypto/ecdh` 做密钥协商
   - `golang.org/x/crypto/hkdf` 派生会话密钥
   - `golang.org/x/crypto/chacha20poly1305` 做 AEAD

3. **长期公钥通过配置文件分发**：在 `config.go` 的 `Config` 中增加 `PeerPublicKey` 字段

4. **临时密钥只在握手 goroutine 内生存**，不存入任何全局状态

```go
// 建议的配置扩展
type Config struct {
    // ... 现有字段 ...
    PeerPublicKey string `json:"peer_public_key" env:"PEDIS_PEER_PUBLIC_KEY"` // base64 编码
}
```
