package cipher

import (
	"crypto/ecdh"
	"crypto/rand"
)

// Identity holds a long-term X25519 key pair.
// Generate once and distribute the public key out-of-band to all peers.
type Identity struct {
	priv *ecdh.PrivateKey
}

// NewIdentity generates a fresh long-term key pair.
func NewIdentity() (*Identity, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv}, nil
}

// ParseIdentity loads a long-term private key from its raw 32-byte encoding.
func ParseIdentity(raw []byte) (*Identity, error) {
	priv, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv}, nil
}

// PublicKey returns the public half for out-of-band distribution.
func (id *Identity) PublicKey() *ecdh.PublicKey {
	return id.priv.PublicKey()
}

// PublicKeyBytes returns the raw 32-byte public key.
func (id *Identity) PublicKeyBytes() []byte {
	return id.priv.PublicKey().Bytes()
}

// PrivateKeyBytes returns the raw 32-byte private key.
// Handle with care — store in a protected file or secret manager.
func (id *Identity) PrivateKeyBytes() []byte {
	return id.priv.Bytes()
}

// ParsePeerPublicKey parses a 32-byte X25519 public key received from a peer.
func ParsePeerPublicKey(raw []byte) (*ecdh.PublicKey, error) {
	return ecdh.X25519().NewPublicKey(raw)
}
