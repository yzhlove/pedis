package cipher

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// pipe returns two net.Conn ends backed by an in-memory TCP loopback.
func pipe(t *testing.T) (client, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	done := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- nil
			return
		}
		done <- c
	}()

	cc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	sc := <-done
	if sc == nil {
		t.Fatal("server accept failed")
	}
	t.Cleanup(func() { cc.Close(); sc.Close() })
	return cc, sc
}

// handshake runs Initiate and Respond concurrently and returns both Sessions.
func handshake(t *testing.T, serverID *Identity) (clientSess, serverSess *Session) {
	t.Helper()
	cConn, sConn := pipe(t)

	errCh := make(chan error, 2)
	sessCh := make(chan *Session, 2)

	clientID, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		s, err := Initiate(cConn, clientID, serverID.PublicKey())
		sessCh <- s
		errCh <- err
	}()
	go func() {
		s, err := Respond(sConn, serverID)
		sessCh <- s
		errCh <- err
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("handshake error: %v", err)
		}
	}
	s1 := <-sessCh
	s2 := <-sessCh
	return s1, s2 // order doesn't matter for tests that cross-validate
}

// ── Identity tests ────────────────────────────────────────────────────────────

func TestNewIdentityRoundTrip(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	id2, err := ParseIdentity(id.PrivateKeyBytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(id.PublicKeyBytes(), id2.PublicKeyBytes()) {
		t.Error("ParseIdentity: public key mismatch")
	}
}

func TestParsePeerPublicKey(t *testing.T) {
	id, _ := NewIdentity()
	pub, err := ParsePeerPublicKey(id.PublicKeyBytes())
	if err != nil {
		t.Fatalf("ParsePeerPublicKey: %v", err)
	}
	if !bytes.Equal(pub.Bytes(), id.PublicKeyBytes()) {
		t.Error("public key bytes mismatch after parse")
	}
}

func TestParsePeerPublicKeyInvalid(t *testing.T) {
	_, err := ParsePeerPublicKey(make([]byte, 10))
	if err == nil {
		t.Error("expected error for invalid public key bytes")
	}
}

// ── Handshake tests ───────────────────────────────────────────────────────────

func TestHandshakeProducesSameSessionKey(t *testing.T) {
	serverID, _ := NewIdentity()
	cConn, sConn := pipe(t)

	clientID, _ := NewIdentity()
	errCh := make(chan error, 2)
	sessCh := make(chan *Session, 2)

	go func() {
		s, err := Initiate(cConn, clientID, serverID.PublicKey())
		sessCh <- s
		errCh <- err
	}()
	go func() {
		s, err := Respond(sConn, serverID)
		sessCh <- s
		errCh <- err
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("handshake: %v", err)
		}
	}

	s1 := <-sessCh
	s2 := <-sessCh

	// Verify keys are equal: message sealed by s1 must open with s2 and vice versa.
	msg := []byte("key agreement check")
	frame := s1.Seal(msg, nil)
	plain, err := s2.Open(frame, nil)
	if err != nil {
		t.Fatalf("cross-open failed: %v", err)
	}
	if !bytes.Equal(plain, msg) {
		t.Errorf("got %q, want %q", plain, msg)
	}
}

func TestHandshakeMITMDetection(t *testing.T) {
	// Server has the real long-term identity.
	serverID, _ := NewIdentity()

	// Attacker has its own identity but pretends to be the server.
	attackerID, _ := NewIdentity()

	cConn, sConn := pipe(t)
	clientID, _ := NewIdentity()

	errCh := make(chan error, 2)
	sessCh := make(chan *Session, 2)

	// Client uses serverID's public key (pre-configured out-of-band).
	go func() {
		s, err := Initiate(cConn, clientID, serverID.PublicKey())
		sessCh <- s
		errCh <- err
	}()
	// Server side uses attacker's identity (wrong long-term key).
	go func() {
		s, err := Respond(sConn, attackerID)
		sessCh <- s
		errCh <- err
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("handshake: %v", err)
		}
	}

	s1 := <-sessCh
	s2 := <-sessCh

	// Sessions derived from mismatched keys must not interoperate.
	msg := []byte("secret data")
	frame := s1.Seal(msg, nil)
	_, err := s2.Open(frame, nil)
	if err == nil {
		t.Error("MITM: attacker session should NOT be able to open client's frame")
	}
}

func TestHandshakeNilIdentity(t *testing.T) {
	serverID, _ := NewIdentity()
	cConn, sConn := pipe(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		Respond(sConn, serverID) //nolint:errcheck
	}()

	_, err := Initiate(cConn, nil, serverID.PublicKey())
	cConn.Close() // unblock server waiting for M1
	<-done

	if err != ErrNilIdentity {
		t.Errorf("expected ErrNilIdentity, got %v", err)
	}
}

func TestHandshakeNilPeerKey(t *testing.T) {
	clientID, _ := NewIdentity()
	var buf bytes.Buffer

	_, err := Initiate(&buf, clientID, (*ecdh.PublicKey)(nil))
	if err != ErrNilPeerKey {
		t.Errorf("expected ErrNilPeerKey, got %v", err)
	}
}

// Each handshake must produce a different session key even for the same identities.
func TestHandshakeFreshKeysEachSession(t *testing.T) {
	serverID, _ := NewIdentity()
	clientID, _ := NewIdentity()

	seal := func() []byte {
		cConn, sConn := pipe(t)
		sessCh := make(chan *Session, 2)

		go func() {
			s, _ := Initiate(cConn, clientID, serverID.PublicKey())
			sessCh <- s
		}()
		go func() {
			s, _ := Respond(sConn, serverID)
			sessCh <- s
		}()

		s := <-sessCh
		<-sessCh
		return s.Seal([]byte("hello"), nil)
	}

	f1 := seal()
	f2 := seal()
	if bytes.Equal(f1, f2) {
		t.Error("two handshakes produced identical frames — ephemeral keys not fresh")
	}
}

// ── Session tests ─────────────────────────────────────────────────────────────

func TestSessionSealOpenRoundTrip(t *testing.T) {
	serverID, _ := NewIdentity()
	c, s := handshake(t, serverID)

	cases := []struct {
		plain []byte
		ad    []byte
	}{
		{[]byte("hello, pedis!"), nil},
		{[]byte("with additional data"), []byte("ad")},
		{make([]byte, 0), nil},
		{make([]byte, 4096), []byte("large")},
	}

	for _, tc := range cases {
		if len(tc.plain) == 4096 {
			io.ReadFull(rand.Reader, tc.plain) //nolint:errcheck
		}
		frame := c.Seal(tc.plain, tc.ad)
		got, err := s.Open(frame, tc.ad)
		if err != nil {
			t.Errorf("Open failed: %v", err)
			continue
		}
		if !bytes.Equal(got, tc.plain) {
			t.Errorf("plaintext mismatch")
		}
	}
}

func TestSessionOverhead(t *testing.T) {
	serverID, _ := NewIdentity()
	c, _ := handshake(t, serverID)

	plain := []byte("overhead test")
	frame := c.Seal(plain, nil)
	if len(frame) != len(plain)+c.Overhead() {
		t.Errorf("frame length %d, want %d", len(frame), len(plain)+c.Overhead())
	}
}

func TestSessionTamperedCiphertext(t *testing.T) {
	serverID, _ := NewIdentity()
	c, s := handshake(t, serverID)

	frame := c.Seal([]byte("tamper me"), nil)

	// Flip every single bit in the ciphertext region and expect auth failure.
	for i := seqSize; i < len(frame); i++ {
		tampered := make([]byte, len(frame))
		copy(tampered, frame)
		tampered[i] ^= 0xFF
		if _, err := s.Open(tampered, nil); err == nil {
			t.Errorf("tampered frame at byte %d accepted", i)
		}
	}
}

func TestSessionTamperedAdditionalData(t *testing.T) {
	serverID, _ := NewIdentity()
	c, s := handshake(t, serverID)

	frame := c.Seal([]byte("ad check"), []byte("correct-ad"))
	_, err := s.Open(frame, []byte("wrong-ad"))
	if err == nil {
		t.Error("wrong additional data should fail authentication")
	}
}

func TestSessionTruncatedFrame(t *testing.T) {
	serverID, _ := NewIdentity()
	c, s := handshake(t, serverID)

	frame := c.Seal([]byte("data"), nil)
	_, err := s.Open(frame[:seqSize+gcmTagSize-1], nil)
	if err == nil {
		t.Error("truncated frame should fail")
	}
}

func TestSessionSequentialMessages(t *testing.T) {
	const N = 100
	serverID, _ := NewIdentity()
	c, s := handshake(t, serverID)

	for i := range N {
		msg := []byte{byte(i)}
		got, err := s.Open(c.Seal(msg, nil), nil)
		if err != nil || !bytes.Equal(got, msg) {
			t.Fatalf("message %d: err=%v got=%v", i, err, got)
		}
	}
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

// pipeB returns an in-memory net.Conn pair suitable for benchmarks.
func pipeB(b *testing.B) (client, server net.Conn) {
	b.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { ln.Close() })

	done := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- nil
			return
		}
		done <- c
	}()

	cc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	sc := <-done
	if sc == nil {
		b.Fatal("server accept failed")
	}
	b.Cleanup(func() { cc.Close(); sc.Close() })
	return cc, sc
}

func BenchmarkHandshake(b *testing.B) {
	serverID, _ := NewIdentity()
	clientID, _ := NewIdentity()

	for b.Loop() {
		cConn, sConn := pipeB(b)
		errCh := make(chan error, 2)
		go func() { _, err := Respond(sConn, serverID); errCh <- err }()
		go func() { _, err := Initiate(cConn, clientID, serverID.PublicKey()); errCh <- err }()
		<-errCh
		<-errCh
	}
}

func BenchmarkSeal1KB(b *testing.B) {
	serverID, _ := NewIdentity()
	cConn, sConn := pipeB(b)

	clientID, _ := NewIdentity()
	sessCh := make(chan *Session, 1)
	go func() {
		Respond(sConn, serverID) //nolint:errcheck
	}()
	sess, err := Initiate(cConn, clientID, serverID.PublicKey())
	if err != nil {
		b.Fatal(err)
	}
	sessCh <- sess

	plain := make([]byte, 1024)
	b.ResetTimer()
	for b.Loop() {
		(<-sessCh).Seal(plain, nil)
		sessCh <- sess
	}
}
