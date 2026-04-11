// Package codec implements the custom encrypted framing protocol used between
// a pedis client and server over a Unix socket.
//
// # Session lifecycle
//
// Every connection goes through three session phases:
//
//	Phase 0 – Bootstrap (S0)
//	  Derived at startup from the server's static public key via HKDF.
//	  Both sides compute S0 independently (NewClient / NewServer → cipher.GetSecret()).
//	  The Auth request is encrypted with S0, so no frame is ever plaintext.
//
//	Phase 1 – Negotiate (S1)
//	  After decoding the Auth request the server computes:
//	    secret_se = server_static_priv.ECDH(client_eph_pub)
//	    S1 = HKDF(client_salt, msgNegotiate, secret_se)
//	  Before decrypting the Auth response the client computes the same S1:
//	    secret_se = client_eph_priv.ECDH(server_static_pub)
//	    S1 = HKDF(client_salt, msgNegotiate, secret_se)
//	  The Auth response is encrypted with S1.
//
//	Phase 2 – Handshake (S2)  ← all subsequent messages
//	  Server derives S2 after sending the Auth response:
//	    secret_ee = server_eph_priv.ECDH(client_eph_pub)
//	    S2 = HKDF(server_salt, msgHandshake, secret_se, secret_ee)
//	  Client derives S2 after verifying the Auth response (respIKE):
//	    secret_ee = client_eph_priv.ECDH(server_eph_pub)
//	    S2 = HKDF(server_salt, msgHandshake, secret_se, secret_ee)
//
// The two HKDF info strings distinguish the two derived sessions so that
// S1 and S2 are cryptographically independent even when the inputs overlap.
package codec

import (
	"net"

	"github.com/yzhlove/pedis/internal/cipher"
	"github.com/yzhlove/pedis/internal/packet"
	"github.com/yzhlove/pedis/internal/text"
	"google.golang.org/protobuf/proto"
)

const (
	msgNegotiate = "pedis-negotiate-message" // HKDF info for S1 (static-ephemeral only)
	msgHandshake = "pedis-handshake-message" // HKDF info for S2 (static-eph + eph-eph)
	msgTimestamp = "pedis-ts-aead"           // HKDF info for one-time timestamp sessions
)

// newTimestampSession derives a one-time AES-256-GCM session from a nanosecond
// timestamp. text.Encode produces a 13-character string; it is expanded to a
// valid 32-byte AES key via HKDF before being passed to NewSession.
func newTimestampSession(ts uint64) (*cipher.Session, error) {
	key, err := cipher.GenerateKey([]byte(text.Encode(ts)), msgTimestamp)
	if err != nil {
		return nil, err
	}
	return cipher.NewSession(key)
}

type (
	reqFunc  func() (proto.Message, error)
	respFunc func(msg proto.Message) error
)

// doHandle executes one request/response round trip:
//  1. Build the request message via req().
//  2. Encode and write it to conn.
//  3. Read and decode the response from conn.
//  4. Process the response via resp().
func doHandle(c ClientCodec, cmd Cmd, conn net.Conn, req reqFunc, resp respFunc) error {
	msg, err := req()
	if err != nil {
		return err
	}
	payload, err := c.Encode(cmd, msg)
	if err != nil {
		return err
	}
	if err = packet.Pack(conn, payload); err != nil {
		return err
	}

	if payload, err = packet.Unpack(conn); err != nil {
		return err
	}

	msg, err = c.Decode(cmd, payload)
	if err != nil {
		return err
	}
	return resp(msg)
}

// Auth performs the IKE-like authentication handshake.
// On success the client session transitions: S0 → S1 (inside Decode) → S2 (inside respIKE).
func Auth(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, AuthCmd, conn, cli.reqAuth, cli.respAuth)
}

// Hello sends the client's name to the server and expects "OK".
// Must be called after Auth (requires S2).
func Hello(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, HelloCmd, conn, cli.reqHello, cli.respHello)
}

// Heartbeat sends a PING and expects a PONG.
// Must be called after Auth (requires S2).
func Heartbeat(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, HeartbeatCmd, conn, cli.reqHeartbeat, cli.respHeartbeat)
}

// Free signals the server that the client wants to detach and enter bridge mode.
// After Free returns the underlying net.Conn carries raw Redis traffic.
func Free(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, FreeCmd, conn, cli.reqFree, cli.respFree)
}
