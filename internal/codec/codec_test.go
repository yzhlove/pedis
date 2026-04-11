package codec

import (
	"crypto/ecdh"
	"crypto/rand"
	"net"
	"os"
	"testing"

	"github.com/yzhlove/pedis/internal/cipher"
	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/text"
)

// TestMain initialises the package-level singletons that the codec depends on:
//   - text module  (text.Encode is called inside reqIKE and server Auth)
//   - cipher identity (provides GetSecret / GetPrivKey / GetPubKey)
func TestMain(m *testing.M) {
	// text.Encode panics if defaultBook is nil; initialise with default settings
	if err := text.New(&config.Config{}).Apply(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// setupCipher configures the cipher module with a freshly generated X25519 key
// pair. SetupTestIdentity sets both GetPrivKey() and GetPubKey() to the same
// key so that client and server can share a single process in tests.
func setupCipher(t *testing.T) {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate X25519 key: %v", err)
	}
	// Salt must be exactly 32 bytes
	salt := []byte("pedis-test-32byte-salt-0123456789")[:32]
	if err := cipher.SetupTestIdentity(priv, salt); err != nil {
		t.Fatalf("SetupTestIdentity: %v", err)
	}
}

// TestClientServerCommunication exercises the full connection lifecycle:
//
//	Auth → Hello → Heartbeat → Free
//
// The server runs in a goroutine and handles each command in a loop until
// FreeCmd is received. The client drives the sequence.
func TestClientServerCommunication(t *testing.T) {
	setupCipher(t)

	// net.Pipe provides a synchronous in-memory full-duplex connection
	srvConn, cliConn := net.Pipe()
	t.Cleanup(func() {
		srvConn.Close()
		cliConn.Close()
	})

	srvErrCh := make(chan error, 1)
	go func() {
		sc, err := NewServer()
		if err != nil {
			srvErrCh <- err
			return
		}
		// serverCodec implements both ServerCodec and ServerHandler
		srv := sc.(*serverCodec)
		for {
			free, err := Handle(srv, srvConn)
			if err != nil {
				srvErrCh <- err
				return
			}
			if free {
				// FreeCmd: server side done; client owns the raw connection
				srvErrCh <- nil
				return
			}
		}
	}()

	cli, err := NewClient("test-client")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Phase 0→1→2: Auth handshake
	if err := Auth(cli, cliConn); err != nil {
		t.Fatalf("Auth: %v", err)
	}

	// Phase 2: Hello (requires S2)
	if err := Hello(cli, cliConn); err != nil {
		t.Fatalf("Hello: %v", err)
	}

	// Phase 2: Heartbeat
	if err := Heartbeat(cli, cliConn); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	// Phase 2: Free – detach and enter bridge mode
	if err := Free(cli, cliConn); err != nil {
		t.Fatalf("Free: %v", err)
	}

	// Confirm the server goroutine exited cleanly
	if err := <-srvErrCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}
