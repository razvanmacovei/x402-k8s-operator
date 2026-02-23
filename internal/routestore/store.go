package routestore

import "sync"

// Store is a thread-safe in-memory route store shared between the controller and gateway.
type Store struct {
	mu     sync.RWMutex
	routes map[string]*CompiledRoute // key: "namespace/name"
}

// New creates a new empty route store.
func New() *Store {
	return &Store{
		routes: make(map[string]*CompiledRoute),
	}
}

// Set adds or updates a compiled route in the store.
func (s *Store) Set(namespace, name string, route *CompiledRoute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[namespace+"/"+name] = route
}

// Delete removes a route from the store.
func (s *Store) Delete(namespace, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.routes, namespace+"/"+name)
}

// Snapshot returns a copy of all routes for safe iteration.
func (s *Store) Snapshot() []*CompiledRoute {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*CompiledRoute, 0, len(s.routes))
	for _, r := range s.routes {
		result = append(result, r)
	}
	return result
}

// Count returns the number of routes in the store.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.routes)
}
