package server

import (
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/redis"
	"github.com/yzhlove/pedis/internal/resp"
)

var (
	errNotRegistered = errors.New("registry: client not registered")
	errTakeTimeout   = errors.New("registry: take timeout")
	errTakeConflict  = errors.New("registry: take already in progress")
)

// takeRequest is sent to a connEntry's goroutine to atomically hand over the conn.
type takeRequest struct {
	connCh chan net.Conn
}

// connEntry holds a managed connection and the channels used to communicate
// with its dedicated goroutine.
type connEntry struct {
	name   string
	conn   net.Conn
	takeCh chan takeRequest
	doneCh chan struct{} // closed only when the goroutine exits due to heartbeat failure
}

// Registry is a thread-safe map of unix client name → managed net.Conn.
// Each registered conn is kept alive via periodic heartbeats. Connections
// that fail a heartbeat are removed automatically.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*connEntry
	stopCh  chan struct{}
	once    sync.Once
}

func newRegistry() *Registry {
	return &Registry{
		entries: make(map[string]*connEntry),
		stopCh:  make(chan struct{}),
	}
}

// Register stores conn under name and starts heartbeat management for it.
// If an existing entry with the same name exists it is closed first (last-one-wins).
func (r *Registry) Register(name string, conn net.Conn) {
	r.mu.Lock()
	if old, ok := r.entries[name]; ok {
		old.conn.Close()
		delete(r.entries, name)
	}
	e := &connEntry{
		name:   name,
		conn:   conn,
		takeCh: make(chan takeRequest, 1),
		doneCh: make(chan struct{}),
	}
	r.entries[name] = e
	r.mu.Unlock()

	go r.runEntry(e)
}

// Get locates the conn registered under name and signals its goroutine to hand
// over ownership. Blocks until the conn is delivered or config.TakeTimeout elapses.
// Returns errNotRegistered if name is unknown, errTakeTimeout if the wait expires.
func (r *Registry) Get(name string) (net.Conn, error) {
	r.mu.Lock()
	e, ok := r.entries[name]
	r.mu.Unlock()
	if !ok {
		return nil, errNotRegistered
	}

	connCh := make(chan net.Conn, 1)
	select {
	case e.takeCh <- takeRequest{connCh: connCh}:
	default:
		return nil, errTakeConflict
	}

	timer := time.NewTimer(config.TakeTimeout)
	defer timer.Stop()

	select {
	case conn := <-connCh:
		return conn, nil
	case <-timer.C:
		return nil, errTakeTimeout
	case <-e.doneCh:
		return nil, errNotRegistered
	}
}

// Close signals all per-conn goroutines to exit, closes every registered
// connection, and empties the registry. Safe to call more than once.
func (r *Registry) Close() {
	r.once.Do(func() { close(r.stopCh) })
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		e.conn.Close()
	}
	clear(r.entries)
	r.entries = nil
}

func (r *Registry) remove(name string) {
	r.mu.Lock()
	delete(r.entries, name)
	r.mu.Unlock()
}

// runEntry is the per-connection goroutine. It drives heartbeats and responds
// to take requests. It exits when the heartbeat fails or the conn is taken.
func (r *Registry) runEntry(e *connEntry) {
	ticker := time.NewTicker(config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			if err := r.heartbeat(e); err != nil {
				log.Error("registry: heartbeat failed, removing client",
					slog.String("name", e.name), log.ErrWrap(err))
				r.remove(e.name)
				e.conn.Close()
				close(e.doneCh)
				return
			}
		case req := <-e.takeCh:
			r.remove(e.name)
			req.connCh <- e.conn
			return
		}
	}
}

func (r *Registry) heartbeat(e *connEntry) error {
	return config.RWConnTimeout(e.conn, func() error {
		if err := redis.PING(e.conn); err != nil {
			return err
		}
		return resp.GetObject(e.conn, func(obj resp.Object) error {
			if obj.Type() != resp.StatusType {
				return errors.New("registry: expected status response to PING")
			}
			if obj.(*resp.Status).Get() != "PONG" {
				return errors.New("registry: expected PONG")
			}
			return nil
		})
	})
}
