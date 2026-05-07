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
	entries map[string][]*connEntry
	stopCh  chan struct{}
	once    sync.Once
}

func newRegistry() *Registry {
	return &Registry{
		entries: make(map[string][]*connEntry),
		stopCh:  make(chan struct{}),
	}
}

// Register stores conn under name and starts heartbeat management for it.
// If an existing entry with the same name exists it is closed first (last-one-wins).
func (r *Registry) Register(name string, conn net.Conn) {
	r.mu.Lock()
	e := &connEntry{
		name:   name,
		conn:   conn,
		takeCh: make(chan takeRequest, 1),
		doneCh: make(chan struct{}),
	}
	r.entries[name] = append(r.entries[name], e)
	r.mu.Unlock()

	go r.runEntry(e)
}

// Get locates an available conn registered under name and signals its goroutine
// to hand over ownership. It iterates the bucket LIFO: on a conflict (another
// Get is already taking that entry) or a dead entry, it falls back to the next
// candidate. Blocks up to config.TakeTimeout on the entry that accepts the
// take request. Returns errNotRegistered if no entry can be taken,
// errTakeTimeout if the accepted entry fails to hand over in time.
func (r *Registry) Get(name string) (net.Conn, error) {
	r.mu.Lock()
	bucket := r.entries[name]
	if len(bucket) == 0 {
		r.mu.Unlock()
		return nil, errNotRegistered
	}
	snap := make([]*connEntry, len(bucket))
	copy(snap, bucket)
	r.mu.Unlock()

	for i := len(snap) - 1; i >= 0; i-- {
		e := snap[i]

		// Skip entries that already died.
		select {
		case <-e.doneCh:
			continue
		default:
		}

		connCh := make(chan net.Conn, 1)
		select {
		case e.takeCh <- takeRequest{connCh: connCh}:
			// Take request accepted; fall through to wait for handoff.
		default:
			// Another Get already holds the take slot; try the next candidate.
			continue
		}

		timer := time.NewTimer(config.TakeTimeout)
		select {
		case conn := <-connCh:
			timer.Stop()
			return conn, nil
		case <-e.doneCh:
			timer.Stop()
			// Entry died before handing over; try the next candidate.
			continue
		case <-timer.C:
			return nil, errTakeTimeout
		}
	}
	return nil, errNotRegistered
}

// Close signals all per-conn goroutines to exit, closes every registered
// connection, and empties the registry. Safe to call more than once.
func (r *Registry) Close() {
	r.once.Do(func() { close(r.stopCh) })
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entries := range r.entries {
		for _, e := range entries {
			e.conn.Close()
		}
	}
	clear(r.entries)
	r.entries = nil
}

// remove removes the specific entry from the name bucket. If the bucket becomes
// empty the key is deleted. Uses swap-remove (O(1), order not preserved).
func (r *Registry) remove(name string, target *connEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := r.entries[name]
	for i, e := range entries {
		if e == target {
			last := len(entries) - 1
			entries[i] = entries[last]
			entries[last] = nil
			entries = entries[:last]
			if len(entries) == 0 {
				delete(r.entries, name)
			} else {
				r.entries[name] = entries
			}
			return
		}
	}
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
				r.remove(e.name, e)
				e.conn.Close()
				close(e.doneCh)
				return
			}
		case req := <-e.takeCh:
			r.remove(e.name, e)
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
