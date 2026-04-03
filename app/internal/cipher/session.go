package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"sync/atomic"
)

// ErrAuthFailed is returned by Open when the ciphertext is invalid or tampered.
var ErrAuthFailed = errors.New("cipher: authentication failed")

const (
	nonceSize  = 12 // AES-GCM standard nonce length
	seqSize    = 8  // sequence-number prefix written before the ciphertext
	gcmTagSize = 16 // AES-GCM authentication tag
)

// Session holds the symmetric cipher state for one established connection.
//
// Each call to Seal increments an outbound sequence counter; the counter value
// is written as an 8-byte big-endian prefix and also used to build the 12-byte
// AES-GCM nonce (4 zero bytes ‖ 8-byte counter).  Open reads the prefix from
// the received frame and reconstructs the nonce without keeping local state,
// making Session safe to use from multiple goroutines as long as the caller
// ensures in-order delivery per direction.
//
// Wire layout of a sealed frame:
//
//	[ seq (8 bytes) | ciphertext (N bytes) | GCM tag (16 bytes) ]
type Session struct {
	aead    cipher.AEAD
	sealSeq atomic.Uint64 // monotonic outbound nonce counter
}

// Overhead returns the number of bytes Seal adds on top of the plaintext.
func (s *Session) Overhead() int { return seqSize + gcmTagSize }

// Seal encrypts and authenticates plaintext.
// additionalData is authenticated but not encrypted (may be nil).
// The returned slice is self-contained and ready to write to the wire.
func (s *Session) Seal(plaintext, additionalData []byte) []byte {
	seq := s.sealSeq.Add(1) - 1
	nonce := buildNonce(seq)

	// Allocate: seq prefix + ciphertext + GCM tag
	out := make([]byte, seqSize+len(plaintext)+gcmTagSize)
	binary.BigEndian.PutUint64(out[:seqSize], seq)
	s.aead.Seal(out[seqSize:seqSize], nonce[:], plaintext, additionalData)
	return out
}

// Open decrypts and verifies a frame produced by Seal.
// additionalData must match what was passed to Seal (may be nil).
func (s *Session) Open(frame, additionalData []byte) ([]byte, error) {
	if len(frame) < seqSize+gcmTagSize {
		return nil, ErrAuthFailed
	}
	seq := binary.BigEndian.Uint64(frame[:seqSize])
	nonce := buildNonce(seq)

	plain, err := s.aead.Open(nil, nonce[:], frame[seqSize:], additionalData)
	if err != nil {
		return nil, ErrAuthFailed
	}
	return plain, nil
}

// newSession wraps a 32-byte AES-256-GCM key in a Session.
func newSession(key []byte) (*Session, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Session{aead: aead}, nil
}

// buildNonce constructs the 12-byte AES-GCM nonce from a sequence number.
// Layout: [ 0x00 0x00 0x00 0x00 | seq (8 bytes big-endian) ]
func buildNonce(seq uint64) [nonceSize]byte {
	var n [nonceSize]byte
	binary.BigEndian.PutUint64(n[4:], seq)
	return n
}
