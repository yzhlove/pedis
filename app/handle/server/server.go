package server

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"time"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/handle"
	"github.com/yzhlove/peids/app/internal/packet"
	session "github.com/yzhlove/peids/app/modules/cipher"
	"github.com/yzhlove/peids/app/modules/text"
	"github.com/yzhlove/peids/app/proto/pb"
	"google.golang.org/protobuf/proto"
)

var (
	errCommand   = errors.New("server: command error")
	errVerify    = errors.New("server: verify error")
	errHeartbeat = errors.New("server: heartbeat error")
)

type server struct {
	defSalt []byte
	msgInfo string
	session *session.Session
}

func New(cfg *config.Config) (*server, error) {
	return &server{}, nil
}

func (s *server) Encode(cmd handle.Cmd, msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))

	if cmd != handle.AuthCmd {
		payload = s.session.Encrypt(payload, nil)
	}
	return payload, nil
}

func (s *server) Decode(payload []byte) (cmd handle.Cmd, msg proto.Message, err error) {

	p := packet.Packet(payload)
	cmd = handle.Cmd(p.Cmd())
	if cmd == handle.UnknownCmd {
		return handle.UnknownCmd, nil, errCommand
	}

	var data = p.Payload()
	if s.session != nil {
		if data, err = s.session.Decrypt(p.Payload(), nil); err != nil {
			return handle.UnknownCmd, nil, err
		}
	}

	switch cmd {
	case handle.AuthCmd:
		msg = new(pb.Auth)
	case handle.HeartbeatCmd:
		msg = new(pb.String)
	}
	err = proto.Unmarshal(data, msg)
	return
}

func (s *server) Auth(msg proto.Message) (proto.Message, error) {

	req := msg.(*pb.Auth)
	ecdsaPub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), req.EcdsaPubKeyBytes)
	if err != nil {
		return nil, err
	}

	if !ecdsa.VerifyASN1(ecdsaPub, append(req.DHPubKeyBytes, req.Salt...), req.Signature) {
		return nil, errVerify
	}

	aead, err := session.NewSession([]byte(text.Encode(req.Timestamp)))
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

	secret2, err := session.GetPrivKey().ECDH(cliPub)
	if err != nil {
		return nil, err
	}

	key, err := session.GenerateKey(salt, s.msgInfo, secret1, secret2)
	if err != nil {
		return nil, err
	}

	// send to client
	resp := new(pb.Auth)
	resp.Timestamp = uint64(time.Now().UnixNano())
	if aead, err = session.NewSession([]byte(text.Encode(resp.Timestamp))); err != nil {
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

	if s.session, err = session.NewSession(key); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *server) Heartbeat(msg proto.Message) (proto.Message, error) {
	req := msg.(*pb.String)
	if req.Data == "PING" {
		resp := new(pb.String)
		resp.Data = "PONG"
		return resp, nil
	}
	return nil, errHeartbeat
}
