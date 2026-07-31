// Package gwopt carries the gateway specific option state that lives inside
// [core.Options], so every gateway package expresses its own options with the
// shared [core.Option] type instead of inventing a parallel one.
package gwopt

import "github.com/amiranmanesh/payvand/core"

// From returns the settings struct stored under key, creating it on first use.
// Gateway packages wrap it in a private helper:
//
//	func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }
func From[T any](o *core.Options, key string) *T {
	if existing, ok := o.Extra(key).(*T); ok {
		return existing
	}
	created := new(T)
	o.SetExtra(key, created)
	return created
}
