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
	"github.com/yzhlove/pedis/internal/text"
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

// replayCache stores recently used Auth timestamps to reject replayed messages.
// Key: uint64 timestamp, Value: int64 expiry (UnixNano).
var replayCache sync.Map

// checkReplay returns errServerVerify if ts is outside the time window or has
// already been used within the window (replay attack).
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

	expiry := now.Add(replayWindow).UnixNano()
	if _, loaded := replayCache.LoadOrStore(ts, expiry); loaded {
		return errServerVerify
	}

	// Lazy cleanup: evict expired entries on each new auth.
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
	Heartbeat(msg proto.Message) (proto.Message, error)
	Free(msg proto.Message) (proto.Message, error)
}

// Handle reads one request from conn, dispatches it, and writes the response.
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

func serverRouter(h ServerHandler, cmd Cmd, msg proto.Message) (ok bool, resp proto.Message, err error) {
	switch cmd {
	case AuthCmd:
		resp, err = h.Auth(msg)
	case HeartbeatCmd:
		resp, err = h.Heartbeat(msg)
	case FreeCmd:
		ok = true
		resp, err = h.Free(msg)
	default:
		err = fmt.Errorf("router: unknown command: %d", cmd)
	}
	return
}

type serverCodec struct {
	secret1 []byte
	secret2 []byte
	salt    []byte
	name    string
	session *cipher.Session
}

// NewServer creates a new server-side codec.
// The bootstrap session encrypts the first RTT using the pre-derived secret
// from cipher.GetSecret(), matching the client's bootstrap session.
func NewServer() (ServerCodec, error) {
	session, err := cipher.NewSession(cipher.GetSecret())
	if err != nil {
		return nil, err
	}
	return &serverCodec{session: session}, nil
}

func (s *serverCodec) GetClientName() string {
	return s.name
}

func (s *serverCodec) encodeWithAuthCmd(cmd Cmd) error {
	if cmd == AuthCmd {
		if len(s.secret1) == 0 || len(s.secret2) == 0 || len(s.salt) == 0 {
			return nil
		}
		key, err := cipher.GenerateKey(s.salt, msgHandshake, s.secret1, s.secret2)
		if err != nil {
			return err
		}
		if s.session, err = cipher.NewSession(key); err != nil {
			return err
		}
		s.secret1, s.secret2, s.salt = nil, nil, nil
	}
	return nil
}

func (s *serverCodec) Encode(cmd Cmd, msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))
	respData := s.session.Encrypt(payload, nil)
	return respData, s.encodeWithAuthCmd(cmd)
}

func (s *serverCodec) Decode(payload []byte) (cmd Cmd, msg proto.Message, err error) {
	// Decrypt the full payload first, then parse the inner frame.
	data, err := s.session.Decrypt(payload, nil)
	if err != nil {
		return UnknownCmd, nil, err
	}

	p := packet.Packet(data)
	cmd = Cmd(p.Cmd())
	if cmd == UnknownCmd {
		return UnknownCmd, nil, errServerCommand
	}

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

func (s *serverCodec) Auth(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.Auth)
	if err := checkReplay(req.Timestamp); err != nil {
		return nil, err
	}

	ecdsaPub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), req.EcdsaPubKeyBytes)
	if err != nil {
		return nil, err
	}

	if !ecdsa.VerifyASN1(ecdsaPub, append(req.DHPubKeyBytes, req.Salt...), req.Signature) {
		return nil, errServerVerify
	}

	aead, err := cipher.NewSession([]byte(text.Encode(req.Timestamp)))
	if err != nil {
		return nil, err
	}

	cliSalt, err := aead.Decrypt(req.Salt, nil)
	if err != nil {
		return nil, err
	}

	dhPubKeyBytes, err := aead.Decrypt(req.DHPubKeyBytes, cliSalt)
	if err != nil {
		return nil, err
	}

	cliPub, err := ecdh.X25519().NewPublicKey(dhPubKeyBytes)
	if err != nil {
		return nil, err
	}

	if s.secret1, err = cipher.GetPrivKey().ECDH(cliPub); err != nil {
		return nil, err
	}
	key, err := cipher.GenerateKey(cliSalt, msgNegotiate, s.secret1)
	if err != nil {
		return nil, err
	}
	if s.session, err = cipher.NewSession(key); err != nil {
		return nil, err
	}

	// Build response. Include an ephemeral DH key and ECDSA signature for
	// response integrity; the client verifies the signature in respIKE.
	resp := new(pb.Auth)
	resp.Timestamp = uint64(time.Now().UnixNano())
	if aead, err = cipher.NewSession([]byte(text.Encode(resp.Timestamp))); err != nil {
		return nil, err
	}

	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	if s.secret2, err = privKey.ECDH(cliPub); err != nil {
		return nil, err
	}

	s.salt = make([]byte, 32)
	if _, err = rand.Read(s.salt); err != nil {
		return nil, err
	}
	resp.Salt = aead.Encrypt(s.salt, nil)
	resp.DHPubKeyBytes = aead.Encrypt(s.salt, privKey.PublicKey().Bytes())

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

func (s *serverCodec) Heartbeat(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.String)
	if req.Data == "PING" {
		return &pb.String{Data: "PONG"}, nil
	}
	return nil, errServerHeartbeat
}

func (s *serverCodec) Hello(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.String)
	if len(req.Data) == 0 {
		return nil, errServerHello
	}
	s.name = req.Data
	return &pb.String{Data: "OK"}, nil
}

func (s *serverCodec) Free(msg proto.Message) (proto.Message, error) {
	return new(pb.Nil), nil
}
