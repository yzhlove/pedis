package cipher

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"io"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/modules"
)

var (
	errServerKey = errors.New("identity: server key is empty! ")
	errClientKey = errors.New("identity: client key is empty! ")
)

var _identity *identity

type identity struct {
	cfg    *config.Config
	priv   *ecdh.PrivateKey
	pub    *ecdh.PublicKey
	prefix [4]byte
}

func New(cfg *config.Config) modules.Modules {
	return &identity{cfg: cfg}
}

func (i *identity) Apply() error {
	if err := i.build(); err != nil {
		return err
	}
	_identity = i
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
	}

	_, err = io.ReadFull(rand.Reader, i.prefix[:])
	return
}

func GetPubKey() *ecdh.PublicKey {
	if _identity != nil {
		return _identity.pub
	}
	return nil
}

func GetPrivKey() *ecdh.PrivateKey {
	if _identity != nil {
		return _identity.priv
	}
	return nil
}

func getPrefix() [4]byte {
	if _identity != nil {
		return _identity.prefix
	}
	return [4]byte{'J', 'A', 'V', 'A'}
}
func GenerateKey(salt []byte, info string, secret ...[]byte) ([]byte, error) {
	var sl int
	for _, s := range secret {
		sl += len(s)
	}

	s := make([]byte, 0, sl)
	for _, t := range secret {
		secret = append(secret, t)
	}
	return hkdf.Key(sha256.New, s, salt, info, 32)
}
