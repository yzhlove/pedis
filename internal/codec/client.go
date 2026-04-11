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
	"github.com/yzhlove/pedis/internal/text"
	"github.com/yzhlove/pedis/proto/pb"
	"google.golang.org/protobuf/proto"
)

var (
	errVerify  = errors.New("client: verify failed")
	errEcho    = errors.New("client: echo failed")
	errHello   = errors.New("client: hello failed")
	errCommand = errors.New("client: command failed")
	errDecode  = errors.New("client: decode failed")
)

// ClientCodec encodes and decodes client-side framed messages.
type ClientCodec interface {
	Encode(cmd Cmd, message proto.Message) ([]byte, error)
	Decode(cmd Cmd, payload []byte) (proto.Message, error)
}

type clientCodec struct {
	privKey *ecdh.PrivateKey
	name    string
	salt    []byte
	secret  []byte
	session *cipher.Session
}

// NewClient creates a new ClientCodec for the given config.
// The bootstrap session encrypts the first RTT using the pre-derived secret
// from cipher.GetSecret(), so no plaintext frames are ever sent.
func NewClient(name string) (ClientCodec, error) {
	session, err := cipher.NewSession(cipher.GetSecret())
	if err != nil {
		return nil, err
	}
	return &clientCodec{session: session, name: name}, nil
}

func (c *clientCodec) decodeWithAuthCmd(cmd Cmd) (err error) {
	if cmd == AuthCmd {
		if c.privKey == nil {
			return errDecode
		}
		if c.secret, err = c.privKey.ECDH(cipher.GetPubKey()); err != nil {
			return err
		}
		var key []byte
		if key, err = cipher.GenerateKey(c.salt, msgNegotiate, c.secret); err != nil {
			return err
		}
		c.session, err = cipher.NewSession(key)
	}
	return
}

func (c *clientCodec) Encode(cmd Cmd, msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))
	return c.session.Encrypt(payload, nil), nil
}

func (c *clientCodec) Decode(cmd Cmd, payload []byte) (proto.Message, error) {

	if err := c.decodeWithAuthCmd(cmd); err != nil {
		return nil, err
	}

	decrypted, err := c.session.Decrypt(payload, nil)
	if err != nil {
		return nil, err
	}

	p := packet.Packet(decrypted)
	if Cmd(p.Cmd()) != cmd {
		return nil, errCommand
	}

	var msg proto.Message
	switch cmd {
	case AuthCmd:
		msg = new(pb.Auth)
	case HeartbeatCmd:
		msg = new(pb.String)
	case FreeCmd:
		msg = new(pb.Nil)
	}

	err = proto.Unmarshal(p.Payload(), msg)
	return msg, err
}

func (c *clientCodec) reqIKE() (proto.Message, error) {
	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	resp := new(pb.Auth)
	resp.Timestamp = uint64(time.Now().UnixNano())
	aead, err := cipher.NewSession([]byte(text.Encode(resp.Timestamp)))
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 32)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	resp.Salt = aead.Encrypt(salt, nil)
	resp.DHPubKeyBytes = aead.Encrypt(salt, privKey.PublicKey().Bytes())

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

	c.privKey, c.salt = privKey, salt
	return resp, nil
}

// respIKE verifies the server's Auth response. The session was already updated
// in Decode to the handshake key derived from static-ephemeral ECDH.
func (c *clientCodec) respIKE(msg proto.Message) error {
	req := msg.(*pb.Auth)

	ecdsaPub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), req.EcdsaPubKeyBytes)
	if err != nil {
		return err
	}

	if !ecdsa.VerifyASN1(ecdsaPub, append(req.DHPubKeyBytes, req.Salt...), req.Signature) {
		return errVerify
	}

	aead, err := cipher.NewSession([]byte(text.Encode(req.Timestamp)))
	if err != nil {
		return err
	}

	salt, err := aead.Decrypt(req.Salt, nil)
	if err != nil {
		return err
	}

	dhPubKeyBytes, err := aead.Decrypt(salt, req.DHPubKeyBytes)
	if err != nil {
		return err
	}

	servePub, err := ecdh.X25519().NewPublicKey(dhPubKeyBytes)
	if err != nil {
		return err
	}

	secret, err := c.privKey.ECDH(servePub)
	if err != nil {
		return err
	}

	key, err := cipher.GenerateKey(salt, msgHandshake, c.secret, secret)
	if err != nil {
		return err
	}

	if c.session, err = cipher.NewSession(key); err != nil {
		return err
	}
	c.privKey, c.salt, c.secret = nil, nil, nil
	return nil
}

func (c *clientCodec) reqEcho() (proto.Message, error) {
	return &pb.String{Data: "PING"}, nil
}

func (c *clientCodec) respEcho(msg proto.Message) error {
	req := msg.(*pb.String)
	if req.Data != "PONG" {
		return errEcho
	}
	return nil
}

func (c *clientCodec) reqHello() (proto.Message, error) {
	return &pb.String{Data: c.name}, nil
}

func (c *clientCodec) respHello(msg proto.Message) error {
	req := msg.(*pb.String)
	if req.Data != "OK" {
		return errHello
	}
	return nil
}

func (c *clientCodec) reqFree() (proto.Message, error)  { return &pb.Nil{}, nil }
func (c *clientCodec) respFree(msg proto.Message) error { return nil }
