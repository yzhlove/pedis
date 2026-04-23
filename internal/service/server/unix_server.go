package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"

	"github.com/yzhlove/pedis/internal/codec"
	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
)

type unixServer struct {
	cfg      *config.Config
	registry *Registry
	listener net.Listener
	ctx      context.Context
}

func newUnixServer(ctx context.Context, cfg *config.Config, reg *Registry) *unixServer {
	return &unixServer{cfg: cfg, registry: reg, ctx: ctx}
}

func (s *unixServer) serve() error {
	// Remove stale socket file from a previous run.
	if err := os.RemoveAll(s.cfg.UnixSocket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", s.cfg.UnixSocket)
	if err != nil {
		return err
	}
	s.listener = ln
	log.Info("unix server: listening", slog.String("socket", s.cfg.UnixSocket))

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
				log.Error("unix server: accept error", log.ErrWrap(err))
				return err
			}
		}
		go s.handleConn(conn)
	}
}

func (s *unixServer) close() {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *unixServer) handleConn(conn net.Conn) {
	// When the server context is cancelled, close this connection immediately
	// so that any blocking codec.Handle call returns.
	go func() {
		<-s.ctx.Done()
		conn.Close()
	}()

	sc, err := codec.NewServer()
	if err != nil {
		log.Error("unix server: create codec failed", log.ErrWrap(err))
		conn.Close()
		return
	}

	srv, ok := sc.(codec.ServerHandler)
	if !ok {
		log.Error("unix server: codec does not implement ServerHandler")
		conn.Close()
		return
	}

	var free bool
	for !free {
		if err = config.RWConnTimeout(conn, func() error {
			free, err = codec.Handle(srv, conn)
			return err
		}); err != nil {
			log.Error("unix server: connection error", log.ErrWrap(err))
			return
		}
	}

	name := srv.GetClientName()
	if name == "" {
		log.Error("unix server: empty client name")
		return
	}
	log.Info("unix server: client hello", slog.String("name", name))
	s.registry.Register(name, conn)
}
