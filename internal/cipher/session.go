package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
)

var errNonceReplay = errors.New("session: nonce replay detected")

const (
	nonceSize       = 12 // standard GCM nonce size (NIST SP 800-38D)
	noncePrefixLen  = 4  // random prefix within the nonce
	nonceCounterLen = 8  // monotonic sequence counter within the nonce
)

// Session holds the symmetric cipher state for one established connection.
type Session struct {
	cipher.AEAD
	counter     atomic.Uint64
	recvCounter atomic.Uint64
}

func (s *Session) buildNonce() []byte {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce[:noncePrefixLen]); err != nil {
		panic(fmt.Errorf("session: generate nonce err:%v", err))
	}
	binary.BigEndian.PutUint64(nonce[noncePrefixLen:], s.counter.Add(1)-1)
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

// Decrypt opens a frame produced by Encrypt and enforces nonce monotonicity
// to prevent replay attacks. Nonces must arrive in strictly increasing counter order.
func (s *Session) Decrypt(ciphertext, additionalData []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, errors.New("session: ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	incoming := binary.BigEndian.Uint64(nonce[noncePrefixLen:])

	for {
		last := s.recvCounter.Load()
		if incoming < last {
			return nil, errNonceReplay
		}
		if s.recvCounter.CompareAndSwap(last, incoming+1) {
			break
		}
	}

	return s.Open(nil, nonce, ciphertext[nonceSize:], additionalData)
}
