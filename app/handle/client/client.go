package client

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"time"

	"github.com/yzhlove/peids/app/handle"
	session "github.com/yzhlove/peids/app/modules/cipher"
	"github.com/yzhlove/peids/app/modules/text"
	"github.com/yzhlove/peids/app/proto/pb"
	"google.golang.org/protobuf/proto"
)

var (
	errVerify = errors.New("client: verify failed")
	errEcho   = errors.New("client: echo failed")
)

type client struct {
	defSalt []byte
	msgInfo string
	privKey *ecdh.PrivateKey
	salt    []byte
	session *session.Session
}

func NewClient() *client {
	return &client{}
}

func (c *client) isAead() bool {
	return c.session != nil
}

func (c *client) Encode(cmd handle.Cmd, msg proto.Message) ([]byte, error) {

	return nil, nil
}

func (c *client) Decode(cmd handle.Cmd, payload []byte) (proto.Message, error) {

	return nil, nil
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
	resp.DHPubKeyBytes = aead.Encrypt(c.defSalt, privKey.PublicKey().Bytes())

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

	if !ecdsa.VerifyASN1(ecdsaPub, req.DHPubKeyBytes, req.Signature) {
		return errVerify
	}

	aead, err := session.NewSession([]byte(text.Encode(req.Timestamp)))
	if err != nil {
		return err
	}

	dhPubKeyBytes, err := aead.Decrypt(c.defSalt, req.DHPubKeyBytes)
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
