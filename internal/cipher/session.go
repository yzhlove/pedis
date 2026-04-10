package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"sync/atomic"
)

// Session holds the symmetric cipher state for one established connection.
type Session struct {
	cipher.AEAD
	counter atomic.Uint64
}

func (s *Session) buildNonce() []byte {
	nonce := make([]byte, 12)
	prefix := getPrefix()
	nonce[0] = prefix[0]
	nonce[1] = prefix[1]
	nonce[2] = prefix[2]
	nonce[3] = prefix[3]
	binary.BigEndian.PutUint64(nonce[4:], s.counter.Add(1)-1)
	return nonce
}

// NewSession wraps a 32-byte AES-256-GCM key in a Session.
func NewSession(key []byte) (*Session, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Session{AEAD: aead}, nil
}

// Encrypt seals plaintext, prepending the nonce to the output.
func (s *Session) Encrypt(plaintext, additionalData []byte) []byte {
	nonce := s.buildNonce()
	return s.Seal(nonce, nonce, plaintext, additionalData)
}

// Decrypt opens a frame produced by Encrypt.
func (s *Session) Decrypt(ciphertext, additionalData []byte) ([]byte, error) {
	return s.Open(nil, ciphertext[:s.NonceSize()], ciphertext[s.NonceSize():], additionalData)
}
