package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/redis"
	"github.com/yzhlove/pedis/internal/resp"
)

var (
	errRedisData = errors.New("redis-server: data error")
)

type redisServer struct {
	cfg      *config.Config
	registry *Registry
	listener net.Listener
	ctx      context.Context
}

func newRedisServer(ctx context.Context, cfg *config.Config, reg *Registry) *redisServer {
	return &redisServer{cfg: cfg, registry: reg, ctx: ctx}
}

func (s *redisServer) serve() error {
	ln, err := net.Listen("tcp", ":"+s.cfg.ServerPort)
	if err != nil {
		return err
	}
	s.listener = ln
	log.Info("redis server: listening", slog.String("port", s.cfg.ServerPort))

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
				log.Error("redis server: accept error", log.ErrWrap(err))
				return err
			}
		}
		go s.handleConn(conn)
	}
}

func (s *redisServer) close() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *redisServer) handleConn(conn net.Conn) {
	defer conn.Close()

	br := bufio.NewReader(conn)

	name, helloFwd, err := s.waitForAuth(conn, br)
	if err != nil {
		log.Error("redis server: auth phase error", log.ErrWrap(err))
		return
	}
	if len(name) == 0 {
		log.Error("redis server: empty client name after auth")
		return
	}

	unixConn, err := s.registry.Get(name)
	if err != nil {
		log.Info("redis server: no unix client for name", slog.String("name", name), log.ErrWrap(err))
		_ = redis.ErrWrap(conn, fmt.Errorf("ERR no unix client connected for %s", name))
		return
	}
	defer unixConn.Close()

	// All writes to conn from now on must go through client to serialize the
	// AUTH-intercept reply with the backend→client copy stream.
	var writeMu sync.Mutex
	client := &lockedWriter{mu: &writeMu, w: conn}

	if helloFwd != nil {
		// HELLO case: forward the HELLO command (AUTH stripped) to the backend so
		// it produces the protocol-correct response (RESP2 array or RESP3 map).
		a := resp.GetArrBulk()
		a.BuildArray(helloFwd)
		_, werr := unixConn.Write(a.ToBytes())
		resp.FreeArrBulk(a)
		if werr != nil {
			log.Error("redis server: forward HELLO failed", log.ErrWrap(werr))
			return
		}
	} else {
		// AUTH case: pedis owns the handshake; reply +OK directly.
		if err = redis.OK(client); err != nil {
			log.Error("redis server: error writing ok to client", log.ErrWrap(err))
			return
		}
	}

	// Bridge: client → backend is RESP-aware (intercepts AUTH, strips AUTH from
	// HELLO); backend → client is a raw copy. Both directions run concurrently;
	// the first error tears down both sides.
	errCh := make(chan error, 2)
	go func() { errCh <- s.forwardFromClient(br, client, unixConn) }()
	go func() {
		_, err := io.Copy(client, unixConn)
		errCh <- err
	}()
	err = <-errCh
	conn.Close()
	unixConn.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		log.Error("redis server: bridge error", log.ErrWrap(err))
	}
	log.Info("redis server: bridge stopped", slog.String("name", name))
}

// forwardFromClient parses RESP commands from br and forwards them to backend.
// It intercepts AUTH (replies +OK to client without forwarding) and strips the
// AUTH u p triplet from HELLO before forwarding.
func (s *redisServer) forwardFromClient(br *bufio.Reader, client io.Writer, backend net.Conn) error {
	for {
		var params []string
		if err := resp.GetObject(br, func(obj resp.Object) error {
			if obj.Type() != resp.ArrBulkType {
				return errRedisData
			}
			src := obj.(*resp.ArrBulk).Get()
			if len(src) == 0 {
				return errRedisData
			}
			params = make([]string, len(src))
			copy(params, src)
			return nil
		}); err != nil {
			return err
		}

		switch strings.ToUpper(params[0]) {
		case "AUTH":
			if err := redis.OK(client); err != nil {
				return err
			}
			continue
		case "HELLO":
			_, params, _ = parseHelloParams(params)
		}

		a := resp.GetArrBulk()
		a.BuildArray(params)
		_, werr := backend.Write(a.ToBytes())
		resp.FreeArrBulk(a)
		if werr != nil {
			return werr
		}
	}
}

// lockedWriter serializes writes to an underlying writer behind a mutex, so
// two goroutines can safely write without interleaving byte sequences.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// waitForAuth loops reading RESP commands until a valid AUTH or HELLO-with-AUTH
// arrives. Invalid or unsupported commands get a RESP error reply, then we wait
// for the next command. Returns:
//   - name:     the resolved client name / auth token.
//   - helloFwd: nil for plain AUTH; for HELLO, the HELLO command with the
//     "AUTH u p" triplet stripped, to be forwarded to the backend.
func (s *redisServer) waitForAuth(conn net.Conn, br *bufio.Reader) (name string, helloFwd []string, err error) {
	for {
		err = config.ReadConnTimeout(conn, func() error {
			return resp.GetObject(br, func(obj resp.Object) error {
				if obj.Type() != resp.ArrBulkType {
					return redis.ErrWrap(conn, errRedisData)
				}
				params := obj.(*resp.ArrBulk).Get()
				if len(params) == 0 {
					return redis.ErrInvalidArguments(conn)
				}
				switch strings.ToUpper(params[0]) {
				case "AUTH":
					if len(params) < 2 {
						return redis.ErrInvalidArguments(conn)
					}
					name = params[len(params)-1]
				case "HELLO":
					n, stripped, hasAuth := parseHelloParams(params)
					if !hasAuth {
						return redis.ErrNoAuth(conn)
					}
					name = n
					helloFwd = stripped
				default:
					return redis.ErrNoAuth(conn)
				}
				return nil
			})
		})
		if err != nil {
			return
		}
		if name != "" {
			return
		}
	}
}

// parseHelloParams parses a HELLO command, extracting the password from any
// embedded "AUTH <user> <pass>" triplet and returning the command with that
// triplet removed (so the remaining HELLO can be forwarded to the backend
// without leaking pedis-specific credentials).
//
// Grammar: HELLO [protover [AUTH username password] [SETNAME clientname]]
func parseHelloParams(params []string) (name string, stripped []string, hasAuth bool) {
	stripped = make([]string, 0, len(params))
	stripped = append(stripped, params[0])
	for i := 1; i < len(params); {
		if i+2 < len(params) && strings.ToUpper(params[i]) == "AUTH" {
			name = params[i+2]
			hasAuth = true
			i += 3
			continue
		}
		stripped = append(stripped, params[i])
		i++
	}
	return
}
