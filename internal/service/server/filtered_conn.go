package server

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/redis"
	"github.com/yzhlove/pedis/internal/resp"
)

// filteredConn wraps a backend net.Conn and interposes on the client→backend
// write path. AUTH commands are intercepted (replied +OK to client, not
// forwarded). HELLO has its AUTH triplet stripped before forwarding.
// All other commands are re-encoded and forwarded unchanged.
//
// Read delegates to the embedded net.Conn — backend→client responses need no
// filtering, so no Read override is required.
type filteredConn struct {
	net.Conn           // backend; Read and base Close delegate here
	client   io.Writer // locked writer back to the Redis client
	pw       *io.PipeWriter
}

func newFilteredConn(backend net.Conn, client io.Writer) *filteredConn {
	pr, pw := io.Pipe()
	fc := &filteredConn{Conn: backend, client: client, pw: pw}
	go fc.filterLoop(pr)
	return fc
}

// Write feeds bytes into the filter pipeline.
// io.Copy uses this signature when copying client→backend.
func (f *filteredConn) Write(p []byte) (int, error) {
	return f.pw.Write(p)
}

// Close closes the write pipe (which causes filterLoop to read EOF and exit)
// and the underlying backend connection.
func (f *filteredConn) Close() error {
	f.pw.Close()
	return f.Conn.Close()
}

func (f *filteredConn) filterLoop(pr *io.PipeReader) {
	// Closing pr when we exit causes any pending pw.Write to return ErrClosedPipe,
	// which unblocks the io.Copy(fc, clientReader) goroutine in handleConn.
	defer pr.Close()

	// Passing *bufio.Reader to resp.GetObject: bufio.NewReader inside will detect
	// it already is a *bufio.Reader with sufficient size and return the same pointer,
	// so buffered data is not lost between calls.
	br := bufio.NewReader(pr)
	for {
		if err := resp.GetObject(br, f.dispatch); err != nil {
			log.Error("filter: loop stopped", log.ErrWrap(err))
			return
		}
	}
}

func (f *filteredConn) dispatch(obj resp.Object) error {
	if obj.Type() != resp.ArrBulkType {
		_, err := f.Conn.Write(obj.ToBytes())
		return err
	}

	params := obj.(*resp.ArrBulk).Get()
	if len(params) == 0 {
		return nil
	}

	switch strings.ToUpper(params[0]) {
	case "AUTH":
		// Intercept: reply +OK to the Redis client; do not forward to backend.
		// Real Redis has no password configured; forwarding AUTH would cause an error.
		return redis.OK(f.client)

	case "HELLO":
		_, stripped, ok := parseHelloParams(params)
		if ok {
			// AUTH triplet removed; forward the cleaned HELLO to backend.
			return f.writeParams(stripped)
		}
		// HELLO without AUTH (e.g. "HELLO 2" for protocol downgrade): forward as-is.
		return f.writeParams(params)

	default:
		return f.writeParams(params)
	}
}

// writeParams re-encodes params as a RESP bulk-string array and writes to the backend.
func (f *filteredConn) writeParams(params []string) error {
	ab := resp.GetArrBulk()
	defer resp.FreeArrBulk(ab)
	ab.BuildArray(params)
	_, err := f.Conn.Write(ab.ToBytes())
	return err
}

// lockedWriter serializes concurrent writes to the underlying writer.
// Used so that both the filter goroutine (AUTH intercept replies) and the
// backend→client copy goroutine can write to the same client connection safely.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// parseHelloParams extracts (name, strippedParams, ok) from a HELLO command's
// param slice. Grammar: HELLO [protover [AUTH username password] [SETNAME name]]
// name is taken from the AUTH password field; stripped has the AUTH triplet
// removed so it can be forwarded to a password-free backend. ok is false when
// no AUTH sub-command is present.
func parseHelloParams(params []string) (name string, stripped []string, ok bool) {
	stripped = []string{params[0]} // always "HELLO"
	i := 1

	// optional protocol version (a bare token, not a keyword)
	if i < len(params) && !isHelloKeyword(params[i]) {
		stripped = append(stripped, params[i])
		i++
	}

	for i < len(params) {
		switch strings.ToUpper(params[i]) {
		case "AUTH":
			if i+2 >= len(params) {
				return // malformed: AUTH without two following tokens
			}
			name = params[i+2] // password is the proxy-level "name"
			i += 3
			ok = true
		case "SETNAME":
			if i+1 < len(params) {
				stripped = append(stripped, params[i], params[i+1])
				i += 2
			} else {
				i++
			}
		default:
			i++
		}
	}
	return
}

func isHelloKeyword(s string) bool {
	kw := strings.ToUpper(s)
	return kw == "AUTH" || kw == "SETNAME"
}
