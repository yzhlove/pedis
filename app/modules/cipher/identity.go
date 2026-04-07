package cipher

import (
	"crypto/ecdh"
	"errors"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/modules"
)

var (
	errServerKey = errors.New("identity: server key is empty! ")
	errClientKey = errors.New("identity: client key is empty! ")
)

var _identity *identity

type identity struct {
	cfg  *config.Config
	priv *ecdh.PrivateKey
	pub  *ecdh.PublicKey
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

		if i.priv, err = ecdh.X25519().NewPrivateKey([]byte(i.cfg.ServerPrivateKey)); err != nil {
			return err
		}
	case config.ClientRole:
		if len(i.cfg.ServerPublicKey) == 0 {
			return errClientKey
		}

		if i.pub, err = ecdh.X25519().NewPublicKey([]byte(i.cfg.ServerPublicKey)); err != nil {
			return err
		}
	}
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
