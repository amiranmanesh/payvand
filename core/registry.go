package core

import (
	"sort"
	"sync"
)

// Factory builds a gateway from a terminal configuration and a set of options.
// Every gateway package registers one from its init function.
type Factory func(cfg Config, opts ...Option) (Gateway, error)

// registry holds the known gateways. It is written at init time and read
// concurrently afterwards, so it is guarded by a mutex.
var (
	registryMu sync.RWMutex
	registry   = map[Name]Factory{}
)

// Register adds a gateway factory under the given name. It panics on a nil
// factory or a duplicate name, because both are programming mistakes that can
// only be introduced at init time.
func Register(name Name, factory Factory) {
	if factory == nil {
		panic("payvand: Register called with a nil factory for " + name.String())
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic("payvand: gateway already registered: " + name.String())
	}
	registry[name] = factory
}

// New builds the gateway registered under name.
//
// It is the dynamic entry point: the gateway is chosen by a value, typically
// read from configuration or from the merchant's row in the database, which is
// what makes swapping providers a data change instead of a code change.
func New(name Name, cfg Config, opts ...Option) (Gateway, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, NewError(name, "new", ErrGatewayNotRegistered)
	}
	return factory(cfg, opts...)
}

// Registered returns the sorted names of all linked gateways.
func Registered() []Name {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]Name, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

// IsRegistered reports whether a gateway is linked into the binary.
func IsRegistered(name Name) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[name]
	return ok
}
