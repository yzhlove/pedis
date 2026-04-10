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
	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/packet"
	"github.com/yzhlove/pedis/internal/text"
	"github.com/yzhlove/pedis/proto/pb"
	"google.golang.org/protobuf/proto"
)

var (
	errServerCommand   = errors.New("server: command error")
	errServerVerify    = errors.New("server: verify error")
	errServerHeartbeat = errors.New("server: heartbeat error")
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
	Auth(msg proto.Message) (proto.Message, error)
	Heartbeat(msg proto.Message) (proto.Message, error)
}

// Handle reads one request from conn, dispatches it, and writes the response.
func Handle(h ServerHandler, conn net.Conn) error {
	sc, ok := h.(ServerCodec)
	if !ok {
		return errors.New("handler must also implement ServerCodec")
	}

	payload, err := packet.Unpack(conn)
	if err != nil {
		return err
	}
	cmd, msg, err := sc.Decode(payload)
	if err != nil {
		return err
	}

	if msg, err = serverRouter(h, cmd, msg); err != nil {
		return err
	}

	if payload, err = sc.Encode(cmd, msg); err != nil {
		return err
	}
	return packet.Pack(conn, payload)
}

func serverRouter(h ServerHandler, cmd Cmd, msg proto.Message) (proto.Message, error) {
	switch cmd {
	case AuthCmd:
		return h.Auth(msg)
	case HeartbeatCmd:
		return h.Heartbeat(msg)
	}
	return nil, fmt.Errorf("router: unknown command: %d", cmd)
}

type serverCodec struct {
	defSalt []byte
	msgInfo string
	session *cipher.Session
}

// NewServer creates a new server-side codec.
func NewServer(cfg *config.Config) (ServerCodec, error) {
	return &serverCodec{}, nil
}

func (s *serverCodec) Encode(cmd Cmd, msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))

	if cmd != AuthCmd {
		payload = s.session.Encrypt(payload, nil)
	}
	return payload, nil
}

func (s *serverCodec) Decode(payload []byte) (cmd Cmd, msg proto.Message, err error) {
	p := packet.Packet(payload)
	cmd = Cmd(p.Cmd())
	if cmd == UnknownCmd {
		return UnknownCmd, nil, errServerCommand
	}

	data := p.Payload()
	if s.session != nil {
		if data, err = s.session.Decrypt(p.Payload(), nil); err != nil {
			return UnknownCmd, nil, err
		}
	}

	switch cmd {
	case AuthCmd:
		msg = new(pb.Auth)
	case HeartbeatCmd:
		msg = new(pb.String)
	}
	err = proto.Unmarshal(data, msg)
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

	salt, err := aead.Decrypt(req.Salt, s.defSalt)
	if err != nil {
		return nil, err
	}

	dhPubKeyBytes, err := aead.Decrypt(req.DHPubKeyBytes, salt)
	if err != nil {
		return nil, err
	}

	cliPub, err := ecdh.X25519().NewPublicKey(dhPubKeyBytes)
	if err != nil {
		return nil, err
	}

	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	secret1, err := privKey.ECDH(cliPub)
	if err != nil {
		return nil, err
	}

	secret2, err := cipher.GetPrivKey().ECDH(cliPub)
	if err != nil {
		return nil, err
	}

	key, err := cipher.GenerateKey(salt, s.msgInfo, secret1, secret2)
	if err != nil {
		return nil, err
	}

	resp := new(pb.Auth)
	resp.Timestamp = uint64(time.Now().UnixNano())
	if aead, err = cipher.NewSession([]byte(text.Encode(resp.Timestamp))); err != nil {
		return nil, err
	}

	cliSalt := make([]byte, 32)
	if _, err = rand.Read(cliSalt); err != nil {
		return nil, err
	}
	resp.Salt = aead.Encrypt(s.defSalt, cliSalt)
	resp.DHPubKeyBytes = aead.Encrypt(cliSalt, privKey.PublicKey().Bytes())

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

	if s.session, err = cipher.NewSession(key); err != nil {
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
