package server

import (
	"net"
	"sync"
)

// Registry is a thread-safe map of unix client name → raw net.Conn.
// Only connections that have completed the full IKE handshake and sent Free
// are stored here — they are immediately bridge-ready.
type Registry struct {
	mu      sync.Mutex
	entries map[string]net.Conn
}

func newRegistry() *Registry {
	return &Registry{entries: make(map[string]net.Conn)}
}

// Register stores conn under name. If an existing entry with the same name
// exists it is closed first (last-one-wins).
func (r *Registry) Register(name string, conn net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.entries[name]; ok {
		old.Close()
	}
	r.entries[name] = conn
}

// Take atomically removes and returns the conn for name, or nil if not found.
func (r *Registry) Take(name string) net.Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	conn, ok := r.entries[name]
	if !ok {
		return nil
	}
	delete(r.entries, name)
	return conn
}

// Close closes all registered connections and empties the registry.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, conn := range r.entries {
		conn.Close()
	}
	r.entries = make(map[string]net.Conn)
}
