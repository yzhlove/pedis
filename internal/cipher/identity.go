package cipher

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base32"
	"errors"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/module"
)

var (
	errServerKey = errors.New("identity: server key is empty! ")
	errClientKey = errors.New("identity: client key is empty! ")
	errSalt      = errors.New("identity: salt is invalid! ")
)

var defaultIdentity *identity

const msgINFO = "pedis-default-secret"

type identity struct {
	cfg    *config.Config
	priv   *ecdh.PrivateKey
	pub    *ecdh.PublicKey
	secret []byte
}

func New(cfg *config.Config) module.Module {
	return &identity{cfg: cfg}
}

func (i *identity) Apply() error {
	if err := i.build(); err != nil {
		return err
	}
	defaultIdentity = i
	return nil
}

func (i *identity) build() (err error) {
	switch i.cfg.Role {
	case config.ServerRole:
		if len(i.cfg.ServerPrivateKey) == 0 {
			return errServerKey
		}
		privateKey, err := base32.StdEncoding.DecodeString(i.cfg.ServerPrivateKey)
		if err != nil {
			return err
		}
		if i.priv, err = ecdh.X25519().NewPrivateKey(privateKey); err != nil {
			return err
		}
		return i.initSecret(i.priv.PublicKey().Bytes())
	case config.ClientRole:
		if len(i.cfg.ServerPublicKey) == 0 {
			return errClientKey
		}
		publicKey, err := base32.StdEncoding.DecodeString(i.cfg.ServerPublicKey)
		if err != nil {
			return err
		}
		if i.pub, err = ecdh.X25519().NewPublicKey(publicKey); err != nil {
			return err
		}
		return i.initSecret(publicKey)
	}
	return
}

func (i *identity) initSecret(data []byte) (err error) {
	if len(i.cfg.Salt) == 0 || len(i.cfg.Salt) != 32 {
		return errSalt
	}
	i.secret, err = GenerateKey([]byte(i.cfg.Salt), msgINFO, data)
	return
}

func GetPubKey() *ecdh.PublicKey {
	if defaultIdentity != nil {
		return defaultIdentity.pub
	}
	return nil
}

func GetPrivKey() *ecdh.PrivateKey {
	if defaultIdentity != nil {
		return defaultIdentity.priv
	}
	return nil
}

func GetSecret() []byte {
	if defaultIdentity != nil {
		return defaultIdentity.secret
	}
	return nil
}

// GenerateKey derives a 32-byte key using HKDF-SHA256.
func GenerateKey(salt []byte, info string, secrets ...[]byte) ([]byte, error) {
	var ikm []byte
	for _, s := range secrets {
		ikm = append(ikm, s...)
	}
	return hkdf.Key(sha256.New, ikm, salt, info, 32)
}
