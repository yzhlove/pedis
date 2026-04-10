package client

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
	errVerify  = errors.New("client: verify failed")
	errEcho    = errors.New("client: echo failed")
	errCommand = errors.New("client: command failed")
)

type client struct {
	defSalt []byte
	msgInfo string
	privKey *ecdh.PrivateKey
	salt    []byte
	session *session.Session
}

func New(cfg *config.Config) (*client, error) {
	return &client{}, nil
}

func (c *client) Encode(cmd handle.Cmd, msg proto.Message) ([]byte, error) {

	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	p := packet.Packet(data)
	payload := p.Pack(uint16(cmd))

	if c.session != nil {
		payload = c.session.Encrypt(payload, nil)
	}
	return payload, nil
}

func (c *client) Decode(cmd handle.Cmd, payload []byte) (msg proto.Message, err error) {

	if c.session != nil {
		if payload, err = c.session.Decrypt(payload, nil); err != nil {
			return nil, err
		}
	}

	p := packet.Packet(payload)
	if handle.Cmd(p.Cmd()) != cmd {
		return nil, errCommand
	}

	switch cmd {
	case handle.AuthCmd:
		msg = new(pb.Auth)
	case handle.HeartbeatCmd:
		msg = new(pb.String)
	}

	err = proto.Unmarshal(p.Payload(), msg)
	return
}

func (c *client) reqIke() (proto.Message, error) {

	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	resp := new(pb.Auth)
	resp.Timestamp = uint64(time.Now().UnixNano())
	aead, err := session.NewSession([]byte(text.Encode(resp.Timestamp)))
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 32)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	resp.Salt = aead.Encrypt(c.defSalt, salt)
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

func (c *client) respIke(msg proto.Message) error {

	req := msg.(*pb.Auth)
	ecdsaPub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), req.EcdsaPubKeyBytes)
	if err != nil {
		return err
	}

	if !ecdsa.VerifyASN1(ecdsaPub, append(req.DHPubKeyBytes, req.Salt...), req.Signature) {
		return errVerify
	}

	aead, err := session.NewSession([]byte(text.Encode(req.Timestamp)))
	if err != nil {
		return err
	}

	salt, err := aead.Decrypt(req.Salt, c.defSalt)
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

	secret1, err := c.privKey.ECDH(servePub)
	if err != nil {
		return err
	}

	secret2, err := c.privKey.ECDH(session.GetPubKey())
	if err != nil {
		return err
	}

	key, err := session.GenerateKey(c.salt, c.msgInfo, secret1, secret2)
	if err != nil {
		return err
	}

	if c.session, err = session.NewSession(key); err != nil {
		return err
	}
	c.privKey, c.salt = nil, nil
	return nil
}

func (c *client) reqEcho() (proto.Message, error) {
	return &pb.String{Data: "PING"}, nil
}

func (c *client) respEcho(msg proto.Message) error {
	req := msg.(*pb.String)
	if req.Data != "PONG" {
		return errEcho
	}
	return nil
}
