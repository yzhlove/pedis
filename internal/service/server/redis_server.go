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
	"github.com/yzhlove/pedis/internal/helper"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/redis"
	"github.com/yzhlove/pedis/internal/resp"
)

// clientSide aggregates the per-direction handles for the Redis-client end of
// the bridge into a single io.ReadWriteCloser:
//   - Read goes through the bufio.Reader so bytes already buffered by the
//     handshake-time parser are not lost when the bridge starts.
//   - Write goes through the lockedWriter so it does not race with AUTH-intercept
//     replies from the filter goroutine.
//   - Close closes the underlying client conn.
type clientSide struct {
	io.Reader
	io.Writer
	io.Closer
}

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
	name, helloParams, err := s.waitForAuth(conn, br)
	if err != nil {
		log.Error("redis server: handshake error", log.ErrWrap(err))
		return
	}

	unixConn, err := s.registry.Get(name)
	if err != nil {
		log.Info("redis server: no unix client for name", slog.String("name", name), log.ErrWrap(err))
		_ = redis.ErrWrap(conn, fmt.Errorf("ERR no unix client connected for %s", name))
		return
	}

	var mu sync.Mutex
	lw := &lockedWriter{mu: &mu, w: conn}

	if helloParams != nil {
		// Forward the stripped HELLO to the backend before starting the bridge.
		// io.Copy(lw, fc) will relay the HELLO Map response → client automatically.
		ab := resp.GetArrBulk()
		ab.BuildArray(helloParams)
		bts := ab.ToBytes()
		resp.FreeArrBulk(ab)
		if _, err = unixConn.Write(bts); err != nil {
			log.Error("redis server: hello forward error", log.ErrWrap(err))
			unixConn.Close()
			return
		}
	} else {
		// AUTH handshake already complete; reply +OK now.
		if err = redis.OK(lw); err != nil {
			log.Error("redis server: ok reply error", log.ErrWrap(err))
			unixConn.Close()
			return
		}
	}

	log.Info("redis server: bridge starting", slog.String("name", name))
	fc := newFilteredConn(unixConn, lw)
	err = helper.Bridge(fc, &clientSide{Reader: br, Writer: lw, Closer: conn})
	log.Info("redis server: bridge stopped", slog.String("name", name), log.ErrWrap(err))
}

// waitForAuth reads from br until it finds an AUTH or a HELLO-with-AUTH command.
// Returns the client "name" (AUTH password) and, for HELLO, the stripped params
// to forward to the backend. Sends RESP errors for unrecognised commands and
// continues looping so the client can retry.
func (s *redisServer) waitForAuth(conn net.Conn, br *bufio.Reader) (name string, helloParams []string, err error) {
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
					switch len(params) {
					case 2: // AUTH password
						name = params[1]
						return redis.OK(conn)
					case 3: // AUTH username password
						name = params[2]
						return redis.OK(conn)
					default:
						return redis.ErrAuthParams(conn)
					}
				case "HELLO":
					n, stripped, ok := parseHelloParams(params)
					if !ok {
						return redis.ErrWrap(conn, fmt.Errorf("ERR HELLO requires AUTH"))
					}
					name = n
					helloParams = stripped
					return nil
				default:
					return redis.ErrWrap(conn, fmt.Errorf("ERR unknown command %q", params[0]))
				}
			})
		})
		if err != nil || name != "" {
			return
		}
	}
}
