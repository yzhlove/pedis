package codec

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yzhlove/pedis/internal/cipher"
	"github.com/yzhlove/pedis/internal/packet"
	"github.com/yzhlove/pedis/proto/pb"
	"google.golang.org/protobuf/proto"
)

var (
	errServerCommand   = errors.New("server: command error")
	errServerVerify    = errors.New("server: verify error")
	errServerHeartbeat = errors.New("server: heartbeat error")
	errServerHello     = errors.New("server: hello error")
)

// replayWindow is the maximum age (and future skew) accepted for Auth timestamps.
const replayWindow = 30 * time.Second

// replayCache stores recently seen Auth timestamps to reject replayed requests.
// Key: uint64 nanosecond timestamp, Value: int64 expiry (UnixNano).
var replayCache sync.Map

// checkReplay rejects timestamps that are outside the acceptance window or
// that have already been used (replay attack). Expired entries are evicted lazily.
func checkReplay(ts uint64) error {
	now := time.Now()
	reqTime := time.Unix(0, int64(ts))
	diff := now.Sub(reqTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > replayWindow {
		return errServerVerify
	}

	// Record this timestamp; reject if it was already seen
	expiry := now.Add(replayWindow).UnixNano()
	if _, loaded := replayCache.LoadOrStore(ts, expiry); loaded {
		return errServerVerify
	}

	// Lazy eviction: remove entries whose window has expired
	nowNano := now.UnixNano()
	replayCache.Range(func(k, v any) bool {
		if v.(int64) < nowNano {
			replayCache.Delete(k)
		}
		return true
	})
	return nil
}

// ServerCodec encodes and decodes server-side framed messages.
type ServerCodec interface {
	Encode(cmd Cmd, message proto.Message) ([]byte, error)
	Decode(payload []byte) (Cmd, proto.Message, error)
}

// ServerHandler processes authenticated commands received by the server.
type ServerHandler interface {
	GetClientName() string
	Auth(msg proto.Message) (proto.Message, error)
	Hello(msg proto.Message) (proto.Message, error)
	Heartbeat(msg proto.Message) (proto.Message, error)
	Free(msg proto.Message) (proto.Message, error)
}

// Handle reads one framed request from conn, dispatches it to the appropriate
// handler method, encodes the response, and writes it back.
// Returns (true, nil) when the client sends FreeCmd (detach / bridge mode).
func Handle(h ServerHandler, conn net.Conn) (bool, error) {
	sc, ok := h.(ServerCodec)
	if !ok {
		return false, errors.New("handler must also implement ServerCodec")
	}

	payload, err := packet.Unpack(conn)
	if err != nil {
		return false, err
	}
	cmd, msg, err := sc.Decode(payload)
	if err != nil {
		return false, err
	}

	if ok, msg, err = serverRouter(h, cmd, msg); err != nil {
		return false, err
	}

	if payload, err = sc.Encode(cmd, msg); err != nil {
		return false, err
	}
	return ok, packet.Pack(conn, payload)
}

// serverRouter dispatches a decoded command to the correct ServerHandler method.
func serverRouter(h ServerHandler, cmd Cmd, msg proto.Message) (ok bool, resp proto.Message, err error) {
	switch cmd {
	case AuthCmd:
		resp, err = h.Auth(msg)
	case HelloCmd:
		resp, err = h.Hello(msg)
	case HeartbeatCmd:
		resp, err = h.Heartbeat(msg)
	case FreeCmd:
		// FreeCmd signals the client wants to detach; ok=true triggers bridge mode
		ok = true
		resp, err = h.Free(msg)
	default:
		err = fmt.Errorf("router: unknown command: %d", cmd)
	}
	return
}

// serverCodec holds all mutable state for one server-side connection.
type serverCodec struct {
	secretSE []byte          // server_static_priv.ECDH(client_eph_pub); stored until S2 is derived
	secretEE []byte          // server_eph_priv.ECDH(client_eph_pub); stored until S2 is derived
	salt     []byte          // server-generated random 32-byte salt included in Auth response
	name     string          // client name received in Hello
	session  *cipher.Session // current AES-256-GCM session; transitions S0 → S1 → S2
}

// NewServer creates a server-side codec initialised with the bootstrap session S0.
// S0 is derived from the server's static public key via HKDF (cipher.GetSecret()),
// matching the S0 the client derives in NewClient.
func NewServer() (ServerCodec, error) {
	// S0: bootstrap session; same key as client's bootstrap session
	session, err := cipher.NewSession(cipher.GetSecret())
	if err != nil {
		return nil, err
	}
	return &serverCodec{session: session}, nil
}

func (s *serverCodec) GetClientName() string { return s.name }

// upgradeHandshakeSession is called by Encode after the Auth response has been
// encrypted with S1. It derives S2 (handshake session) and replaces the
// current session, so all subsequent messages use S2.
//
// S2 derivation (server side):
//
//	S2 = HKDF(s.salt, msgHandshake, secretSE, secretEE)
func (s *serverCodec) upgradeHandshakeSession(cmd Cmd) error {
	if cmd != AuthCmd {
		return nil
	}
	// Guard: secrets are only present if Auth() ran successfully
	if len(s.secretSE) == 0 || len(s.secretEE) == 0 || len(s.salt) == 0 {
		return nil
	}
	// Derive the final handshake session from both shared secrets
	key, err := cipher.GenerateKey(s.salt, msgHandshake, s.secretSE, s.secretEE)
	if err != nil {
		return err
	}
	if s.session, err = cipher.NewSession(key); err != nil {
		return err
	}
	// Clear ephemeral material
	s.secretSE, s.secretEE, s.salt = nil, nil, nil
	return nil
}

// Encode marshals msg into the inner frame [2B cmd | proto bytes], encrypts it
// with the current session, and for AuthCmd upgrades to S2 afterwards.
func (s *serverCodec) Encode(cmd Cmd, msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))

	// Encrypt the inner frame with the current session before switching sessions
	respData := s.session.Encrypt(payload, nil)

	// For AuthCmd: after encrypting the response with S1, upgrade to S2
	return respData, s.upgradeHandshakeSession(cmd)
}

// Decode decrypts the raw wire payload and unmarshals the inner frame.
// The full ciphertext is decrypted first; the cmd header is inside the plaintext.
func (s *serverCodec) Decode(payload []byte) (cmd Cmd, msg proto.Message, err error) {
	// Decrypt the complete payload before reading the command byte
	data, err := s.session.Decrypt(payload, nil)
	if err != nil {
		return UnknownCmd, nil, err
	}

	// Parse inner frame: [2B cmd | proto payload]
	p := packet.Packet(data)
	cmd = Cmd(p.Cmd())
	if cmd == UnknownCmd {
		return UnknownCmd, nil, errServerCommand
	}

	// Allocate the concrete proto type expected for each command
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
	return
}

// Auth processes the client's IKE Auth request and builds the server's response.
//
// Flow:
//  1. Replay check and ECDSA signature verification.
//  2. Decrypt clientSalt and clientEphPub via the timestamp AEAD (reqAead).
//  3. Derive S1 (negotiate session):
//     secretSE = server_static_priv.ECDH(clientEphPub)
//     S1 = HKDF(clientSalt, msgNegotiate, secretSE)
//  4. Build response: generate a server ephemeral DH key, encrypt it with a
//     fresh timestamp AEAD (respAead), and sign the blobs with an ephemeral ECDSA key.
//  5. Store secretEE = serverEphPriv.ECDH(clientEphPub) and server salt for
//     the S2 derivation that happens in upgradeHandshakeSession after the response is sent.
func (s *serverCodec) Auth(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.Auth)

	// Step 1: replay protection
	if err := checkReplay(req.Timestamp); err != nil {
		return nil, err
	}

	// Step 1: verify integrity of the client's encrypted blobs
	ecdsaPub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), req.EcdsaPubKeyBytes)
	if err != nil {
		return nil, err
	}
	if !ecdsa.VerifyASN1(ecdsaPub, append(req.DHPubKeyBytes, req.Salt...), req.Signature) {
		return nil, errServerVerify
	}

	// Step 2: reqAead keyed on the client's timestamp to unwrap the DH material
	reqAead, err := newTimestampSession(req.Timestamp)
	if err != nil {
		return nil, err
	}
	// Decrypt client's random salt (encrypted with nil AAD)
	clientSalt, err := reqAead.Decrypt(req.Salt, nil)
	if err != nil {
		return nil, err
	}
	// Decrypt client's ephemeral DH public key (encrypted with clientSalt as AAD)
	clientEphPubBytes, err := reqAead.Decrypt(req.DHPubKeyBytes, clientSalt)
	if err != nil {
		return nil, err
	}
	clientEphPub, err := ecdh.X25519().NewPublicKey(clientEphPubBytes)
	if err != nil {
		return nil, err
	}

	// Step 3: derive S1 from the static-ephemeral shared secret
	if s.secretSE, err = cipher.GetPrivKey().ECDH(clientEphPub); err != nil {
		return nil, err
	}
	key, err := cipher.GenerateKey(clientSalt, msgNegotiate, s.secretSE)
	if err != nil {
		return nil, err
	}
	// S1 is now active; the response will be encrypted with it
	if s.session, err = cipher.NewSession(key); err != nil {
		return nil, err
	}

	// Step 4: build the Auth response
	resp := new(pb.Auth)
	resp.Timestamp = uint64(time.Now().UnixNano())

	// respAead: one-time session keyed on the response timestamp
	respAead, err := newTimestampSession(resp.Timestamp)
	if err != nil {
		return nil, err
	}

	// Server ephemeral X25519 key for the response
	serverEphPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Step 5: secretEE for the eventual S2 derivation in upgradeHandshakeSession
	if s.secretEE, err = serverEphPriv.ECDH(clientEphPub); err != nil {
		return nil, err
	}

	// Random server-side salt included in the response
	s.salt = make([]byte, 32)
	if _, err = rand.Read(s.salt); err != nil {
		return nil, err
	}
	resp.Salt = respAead.Encrypt(s.salt, nil)
	// Encrypt server's ephemeral DH pub key with server salt as AAD
	resp.DHPubKeyBytes = respAead.Encrypt(serverEphPriv.PublicKey().Bytes(), s.salt)

	// Ephemeral ECDSA key: signs the encrypted blobs for integrity
	ecdsaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if resp.Signature, err = ecdsa.SignASN1(rand.Reader, ecdsaPriv, append(resp.DHPubKeyBytes, resp.Salt...)); err != nil {
		return nil, err
	}
	if resp.EcdsaPubKeyBytes, err = ecdsaPriv.PublicKey.Bytes(); err != nil {
		return nil, err
	}
	return resp, nil
}

// Hello records the client's self-reported name and acknowledges with "OK".
func (s *serverCodec) Hello(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.String)
	if len(req.Data) == 0 {
		return nil, errServerHello
	}
	s.name = req.Data
	return &pb.String{Data: "OK"}, nil
}

// Heartbeat responds to a PING with a PONG.
func (s *serverCodec) Heartbeat(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.String)
	if req.Data == "PING" {
		return &pb.String{Data: "PONG"}, nil
	}
	return nil, errServerHeartbeat
}

// Free acknowledges the client's request to detach and enter bridge mode.
func (s *serverCodec) Free(msg proto.Message) (proto.Message, error) {
	return new(pb.Nil), nil
}
