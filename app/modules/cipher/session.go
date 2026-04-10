package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"sync/atomic"
)

type Session struct {
	cipher.AEAD
	counter atomic.Uint64
}

func (s *Session) buildNonoc() []byte {
	nonce := make([]byte, 12)
	prefix := getPrefix()
	nonce[0] = prefix[0]
	nonce[1] = prefix[1]
	nonce[2] = prefix[2]
	nonce[3] = prefix[3]
	binary.BigEndian.PutUint64(nonce[4:], s.counter.Add(1)-1)
	return nonce
}

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

func (s *Session) Encrypt(plaintext, salt []byte) []byte {
	nonce := s.buildNonoc()
	return s.Seal(nonce, nonce, plaintext, salt)
}

func (s *Session) Decrypt(ciphertext, salt []byte) ([]byte, error) {
	return s.Open(nil, ciphertext[:s.NonceSize()], ciphertext[s.NonceSize():], salt)
}
