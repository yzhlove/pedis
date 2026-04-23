package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/helper"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/redis"
	"github.com/yzhlove/pedis/internal/resp"
)

var (
	errRedisData        = errors.New("redis-server: data error")
	errRedisParamsEmpty = errors.New("redis-server: params empty")
	errRedisNameEmpty   = errors.New("redis-server: name empty")
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

	var (
		name string
		err  error
	)

	if err = config.ReadConnTimeout(conn, func() error {
		if name, err = s.waitForAuth(conn); err == nil {
			if len(name) == 0 {
				err = errRedisNameEmpty
			}
		}
		return err
	}); err != nil {
		log.Error("redis server: auth phase error", log.ErrWrap(err))
		return
	}

	unixConn, err := s.registry.Get(name)
	if err != nil {
		log.Info("redis server: no unix client for name", slog.String("name", name), log.ErrWrap(err))
		if err = redis.ErrWrap(conn, fmt.Errorf("ERR no unix client connected for %s", name)); err != nil {
			log.Error("redis server: error writing error to client", log.ErrWrap(err))
			return
		}
		return
	}

	if err = redis.OK(conn); err != nil {
		log.Error("redis server: error writing ok to client", log.ErrWrap(err))
		return
	}

	if err = helper.Bridge(conn, unixConn); err != nil {
		log.Error("redis server: bridge error", log.ErrWrap(err))
	}
	log.Info("redis server: bridge stopped", slog.String("name", name))
}

// waitForAuth reads RESP2 commands until AUTH is received.
// Returns the resolved client name on success.
func (s *redisServer) waitForAuth(conn net.Conn) (name string, err error) {
	err = resp.GetObject(conn, func(obj resp.Object) error {
		if obj.Type() != resp.ArrBulkType {
			if err := redis.ErrWrap(conn, errRedisData); err != nil {
				return err
			}
			return errRedisData
		}

		params := obj.(*resp.ArrBulk).Get()
		if len(params) == 0 {
			if err = redis.ErrInvalidArguments(conn); err != nil {
				return err
			}
			return errRedisParamsEmpty
		}

		switch strings.ToUpper(params[0]) {
		case "AUTH":
			if len(params) > 0 {
				name = params[len(params)-1]
			}
		case "HELLO":
			if len(params) == 5 && strings.ToUpper(params[2]) == "AUTH" {
				name = params[4]
			}
		default:
			if err = redis.ErrNoAuth(conn); err != nil {
				return err
			}
			return fmt.Errorf("redis-server: command not supported: %s", params[0])
		}
		return nil
	})
	return
}
