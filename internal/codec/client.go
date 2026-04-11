package codec

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"time"

	"github.com/yzhlove/pedis/internal/cipher"
	"github.com/yzhlove/pedis/internal/packet"
	"github.com/yzhlove/pedis/proto/pb"
	"google.golang.org/protobuf/proto"
)

var (
	errVerify    = errors.New("client: verify failed")
	errHeartbeat = errors.New("client: heartbeat failed")
	errHello     = errors.New("client: hello failed")
	errCommand   = errors.New("client: command failed")
	errDecode    = errors.New("client: decode failed")
)

// ClientCodec encodes and decodes client-side framed messages.
type ClientCodec interface {
	Encode(cmd Cmd, message proto.Message) ([]byte, error)
	Decode(cmd Cmd, payload []byte) (proto.Message, error)
}

// clientCodec holds all mutable state for one client connection.
// Fields are zeroed/cleared after each handshake phase completes.
type clientCodec struct {
	ephPriv  *ecdh.PrivateKey // ephemeral X25519 key generated in reqAuth; cleared after respAuth
	name     string           // client name sent in Hello
	salt     []byte           // random 32-byte salt generated in reqAuth; cleared after respAuth
	secretSE []byte           // client_eph_priv.ECDH(server_static_pub); cleared after respAuth
	session  *cipher.Session  // current AES-256-GCM session; transitions S0 → S1 → S2
}

// NewClient creates a client-side codec initialised with the bootstrap session S0.
// S0 is derived from the server's static public key via HKDF (cipher.GetSecret()),
// so the Auth request is never sent in plaintext.
func NewClient(name string) (ClientCodec, error) {
	// S0: bootstrap session shared by both sides before any key exchange
	session, err := cipher.NewSession(cipher.GetSecret())
	if err != nil {
		return nil, err
	}
	return &clientCodec{session: session, name: name}, nil
}

// upgradeNegotiateSession upgrades the session to S1 (negotiate) before the
// Auth response is decrypted. This must run before Decrypt because the server
// already switched to S1 when it encrypted that response.
//
// S1 derivation (client side):
//
//	secretSE = client_eph_priv.ECDH(server_static_pub)
//	S1 = HKDF(client_salt, msgNegotiate, secretSE)
func (c *clientCodec) upgradeNegotiateSession(cmd Cmd) (err error) {
	if cmd != AuthCmd {
		return nil
	}
	if c.ephPriv == nil {
		// ephPriv is only nil if reqAuth was never called – programming error
		return errDecode
	}
	// Compute the static-ephemeral shared secret (matches server's secretSE)
	if c.secretSE, err = c.ephPriv.ECDH(cipher.GetPubKey()); err != nil {
		return err
	}
	// Derive S1 using the same salt and info string as the server
	var key []byte
	if key, err = cipher.GenerateKey(c.salt, msgNegotiate, c.secretSE); err != nil {
		return err
	}
	c.session, err = cipher.NewSession(key)
	return
}

// Encode marshals msg, wraps it in the inner frame [2B cmd | proto bytes],
// and encrypts the whole frame with the current session.
func (c *clientCodec) Encode(cmd Cmd, msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	// Pack adds the 2-byte command prefix to the serialised proto payload
	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))
	// Encrypt the entire inner frame; the nonce is prepended by the session
	return c.session.Encrypt(payload, nil), nil
}

// Decode decrypts payload and unmarshals the inner frame.
// For AuthCmd it first upgrades the session to S1 before decrypting, because
// the server encrypted the Auth response with S1.
func (c *clientCodec) Decode(cmd Cmd, payload []byte) (proto.Message, error) {
	// Switch to S1 before decrypting the Auth response (no-op for other cmds)
	if err := c.upgradeNegotiateSession(cmd); err != nil {
		return nil, err
	}

	// Decrypt the full ciphertext first; cmd header is inside the plaintext
	decrypted, err := c.session.Decrypt(payload, nil)
	if err != nil {
		return nil, err
	}

	// Parse the inner frame: [2B cmd | proto payload]
	p := packet.Packet(decrypted)
	if Cmd(p.Cmd()) != cmd {
		return nil, errCommand
	}

	// Allocate the concrete proto type expected for each command
	var msg proto.Message
	switch cmd {
	case AuthCmd:
		msg = new(pb.Auth)
	case HeartbeatCmd:
		msg = new(pb.String)
	case HelloCmd:
		msg = new(pb.String)
	case FreeCmd:
		msg = new(pb.Nil)
	}

	err = proto.Unmarshal(p.Payload(), msg)
	return msg, err
}

// reqAuth builds the Auth request for the IKE-like handshake.
//
// The request carries:
//   - Timestamp        – nanosecond Unix time; used as replay-protection nonce
//     and as the key for the inner AEAD (tsAead).
//   - Salt             – tsAead.Encrypt(clientSalt, nil)
//   - DHPubKeyBytes    – tsAead.Encrypt(ephemeral_X25519_pub, clientSalt)
//   - EcdsaPubKeyBytes – ephemeral ECDSA-P256 public key (plaintext)
//   - Signature        – ECDSA sign(DHPubKeyBytes ‖ Salt) for integrity
func (c *clientCodec) reqAuth() (proto.Message, error) {
	// Generate the ephemeral X25519 key pair for this connection
	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	req := new(pb.Auth)
	req.Timestamp = uint64(time.Now().UnixNano())

	// tsAead: one-time session keyed on the timestamp; wraps the DH material
	tsAead, err := newTimestampSession(req.Timestamp)
	if err != nil {
		return nil, err
	}

	// Random 32-byte per-connection salt; also serves as AAD for DHPubKeyBytes
	clientSalt := make([]byte, 32)
	if _, err = rand.Read(clientSalt); err != nil {
		return nil, err
	}
	req.Salt = tsAead.Encrypt(clientSalt, nil)
	// Encrypt the ephemeral DH public key with clientSalt as additional data
	req.DHPubKeyBytes = tsAead.Encrypt(ephPriv.PublicKey().Bytes(), clientSalt)

	// Ephemeral ECDSA key pair: signs the encrypted blobs for integrity
	ecdsaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if req.Signature, err = ecdsa.SignASN1(rand.Reader, ecdsaPriv, append(req.DHPubKeyBytes, req.Salt...)); err != nil {
		return nil, err
	}
	if req.EcdsaPubKeyBytes, err = ecdsaPriv.PublicKey.Bytes(); err != nil {
		return nil, err
	}

	// Store ephemeral key and salt; both are cleared in respAuth after S2 is set
	c.ephPriv, c.salt = ephPriv, clientSalt
	return req, nil
}

// respAuth processes the server's Auth response.
// At this point the session is already S1 (set by upgradeNegotiateSession).
// respAuth verifies the server's ECDSA signature, extracts the server's
// ephemeral DH public key, and upgrades the session to S2 (handshake).
//
// S2 derivation (client side):
//
//	secretEE = client_eph_priv.ECDH(server_eph_pub)
//	S2 = HKDF(server_salt, msgHandshake, c.secretSE, secretEE)
func (c *clientCodec) respAuth(msg proto.Message) error {
	resp := msg.(*pb.Auth)

	// Verify integrity of the server's encrypted blobs
	ecdsaPub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), resp.EcdsaPubKeyBytes)
	if err != nil {
		return err
	}
	if !ecdsa.VerifyASN1(ecdsaPub, append(resp.DHPubKeyBytes, resp.Salt...), resp.Signature) {
		return errVerify
	}

	// tsAead: one-time session keyed on the server's response timestamp
	tsAead, err := newTimestampSession(resp.Timestamp)
	if err != nil {
		return err
	}

	// Decrypt server's random salt (encrypted with nil AAD)
	serverSalt, err := tsAead.Decrypt(resp.Salt, nil)
	if err != nil {
		return err
	}
	// Decrypt server's ephemeral DH public key (encrypted with serverSalt as AAD)
	serverEphPubBytes, err := tsAead.Decrypt(resp.DHPubKeyBytes, serverSalt)
	if err != nil {
		return err
	}

	serverEphPub, err := ecdh.X25519().NewPublicKey(serverEphPubBytes)
	if err != nil {
		return err
	}

	// secretEE: ephemeral-ephemeral shared secret (forward secrecy)
	secretEE, err := c.ephPriv.ECDH(serverEphPub)
	if err != nil {
		return err
	}

	// Derive S2: combines static-ephemeral (c.secretSE) and ephemeral-ephemeral (secretEE)
	key, err := cipher.GenerateKey(serverSalt, msgHandshake, c.secretSE, secretEE)
	if err != nil {
		return err
	}
	if c.session, err = cipher.NewSession(key); err != nil {
		return err
	}

	// Clear all ephemeral material now that S2 is established
	c.ephPriv, c.salt, c.secretSE = nil, nil, nil
	return nil
}

func (c *clientCodec) reqHeartbeat() (proto.Message, error) {
	return &pb.String{Data: "PING"}, nil
}

func (c *clientCodec) respHeartbeat(msg proto.Message) error {
	resp := msg.(*pb.String)
	if resp.Data != "PONG" {
		return errHeartbeat
	}
	return nil
}

func (c *clientCodec) reqHello() (proto.Message, error) {
	return &pb.String{Data: c.name}, nil
}

func (c *clientCodec) respHello(msg proto.Message) error {
	resp := msg.(*pb.String)
	if resp.Data != "OK" {
		return errHello
	}
	return nil
}

func (c *clientCodec) reqFree() (proto.Message, error)  { return &pb.Nil{}, nil }
func (c *clientCodec) respFree(msg proto.Message) error { return nil }
