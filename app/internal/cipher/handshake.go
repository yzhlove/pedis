// Package cipher implements an authenticated key-exchange handshake based on
// the "pre-distributed long-term public key" scheme (WireGuard-style Method One).
//
// Protocol overview
//
//	Long-term keys (out-of-band):
//	  Server: long-term private b_s, public B_s
//	  Client: holds B_s (server's public key, pre-configured)
//
//	Per-connection ephemeral keys (generated here, discarded on return):
//	  Client: a_e  →  A_e = a_e·G
//	  Server: b_e  →  B_e = b_e·G
//
//	Wire messages (total 96 bytes on the wire):
//	  M1 (client → server, 64 bytes): [ A_e (32) | salt (32) ]
//	  M2 (server → client, 32 bytes): [ B_e (32) ]
//
//	DH computations:
//	  dh_ee = X25519(a_e, B_e)  ←→  X25519(b_e, A_e)   forward secrecy
//	  dh_es = X25519(a_e, B_s)  ←→  X25519(b_s, A_e)   server authentication
//
//	Session key derivation:
//	  IKM  = dh_ee ‖ dh_es
//	  key  = HKDF-SHA256(IKM, salt, info="pedis-cipher-v1", len=32)
//
// Why MITM fails:
//
//	An attacker intercepting the wire cannot compute dh_es = X25519(a_e, B_s)
//	without knowing b_s (the server's long-term private key).  The client's
//	dh_es and the server's dh_es will therefore differ, producing mismatched
//	session keys and failing the first AEAD decryption.
package cipher

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

const (
	pubKeyLen     = 32 // X25519 public-key wire size
	saltLen       = 32 // HKDF salt size
	sessionKeyLen = 32 // AES-256 key size

	m1Len = pubKeyLen + saltLen // 64 bytes: client → server
	m2Len = pubKeyLen           // 32 bytes: server → client
)

const hkdfInfo = "pedis-cipher-v1"

var (
	ErrNilIdentity = errors.New("cipher: identity must not be nil")
	ErrNilPeerKey  = errors.New("cipher: peer public key must not be nil")
)

// Initiate runs the client side of the handshake over rw.
//
//   - self     is the client's Identity (used only to hold the ephemeral key
//     here; a nil Identity is invalid — the client must have an identity so the
//     function signature stays consistent with Respond).
//   - peerPub  is the server's long-term public key obtained out-of-band.
//
// The ephemeral private key lives only on the call stack and is never stored.
// On success the returned *Session is ready for Seal/Open calls.
func Initiate(rw io.ReadWriter, self *Identity, peerPub *ecdh.PublicKey) (*Session, error) {
	if self == nil {
		return nil, ErrNilIdentity
	}
	if peerPub == nil {
		return nil, ErrNilPeerKey
	}

	// ── Step 1: generate ephemeral key pair (stack-local, GC'd on return) ──
	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// ── Step 2: generate fresh per-session salt ─────────────────────────────
	var salt [saltLen]byte
	if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
		return nil, err
	}

	// ── Step 3: send M1 ─────────────────────────────────────────────────────
	var m1 [m1Len]byte
	copy(m1[:pubKeyLen], ephPriv.PublicKey().Bytes())
	copy(m1[pubKeyLen:], salt[:])
	if _, err := rw.Write(m1[:]); err != nil {
		return nil, err
	}

	// ── Step 4: receive M2 ──────────────────────────────────────────────────
	var m2 [m2Len]byte
	if _, err := io.ReadFull(rw, m2[:]); err != nil {
		return nil, err
	}
	serverEphPub, err := ecdh.X25519().NewPublicKey(m2[:])
	if err != nil {
		return nil, err
	}

	// ── Step 5: DH computations ─────────────────────────────────────────────
	// dh_ee = X25519(a_e, B_e)  — forward secrecy
	dhEE, err := ephPriv.ECDH(serverEphPub)
	if err != nil {
		return nil, err
	}
	// dh_es = X25519(a_e, B_s)  — server authentication
	dhES, err := ephPriv.ECDH(peerPub)
	if err != nil {
		return nil, err
	}

	return deriveSession(dhEE, dhES, salt[:])
}

// Respond runs the server side of the handshake over rw.
//
// self is the server's long-term Identity.  The server's ephemeral private key
// lives only on the call stack and is never stored.
func Respond(rw io.ReadWriter, self *Identity) (*Session, error) {
	if self == nil {
		return nil, ErrNilIdentity
	}

	// ── Step 1: receive M1 ──────────────────────────────────────────────────
	var m1 [m1Len]byte
	if _, err := io.ReadFull(rw, m1[:]); err != nil {
		return nil, err
	}
	clientEphPub, err := ecdh.X25519().NewPublicKey(m1[:pubKeyLen])
	if err != nil {
		return nil, err
	}
	salt := m1[pubKeyLen:]

	// ── Step 2: generate server ephemeral key pair ───────────────────────────
	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// ── Step 3: send M2 ─────────────────────────────────────────────────────
	if _, err := rw.Write(ephPriv.PublicKey().Bytes()); err != nil {
		return nil, err
	}

	// ── Step 4: DH computations ─────────────────────────────────────────────
	// dh_ee = X25519(b_e, A_e)  — forward secrecy  (mirrors client's dh_ee)
	dhEE, err := ephPriv.ECDH(clientEphPub)
	if err != nil {
		return nil, err
	}
	// dh_es = X25519(b_s, A_e)  — server authentication (mirrors client's dh_es)
	// Client computes X25519(a_e, B_s); server computes X25519(b_s, A_e).
	// ECDH commutativity guarantees both yield the same 32-byte secret.
	dhES, err := self.priv.ECDH(clientEphPub)
	if err != nil {
		return nil, err
	}

	return deriveSession(dhEE, dhES, salt)
}

// deriveSession mixes the two DH outputs through HKDF and builds a Session.
func deriveSession(dhEE, dhES, salt []byte) (*Session, error) {
	// IKM = dh_ee ‖ dh_es
	ikm := make([]byte, 0, len(dhEE)+len(dhES))
	ikm = append(ikm, dhEE...)
	ikm = append(ikm, dhES...)

	key, err := hkdf.Key(sha256.New, ikm, salt, hkdfInfo, sessionKeyLen)
	if err != nil {
		return nil, err
	}
	return newSession(key)
}
