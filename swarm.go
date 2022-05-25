// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/p2p/net/swarm.
package swarm

import (
	"time"

	"github.com/libp2p/go-libp2p/p2p/net/swarm"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
)

// ErrSwarmClosed is returned when one attempts to operate on a closed swarm.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrSwarmClosed instead.
var ErrSwarmClosed = swarm.ErrSwarmClosed

// ErrAddrFiltered is returned when trying to register a connection to a
// filtered address. You shouldn't see this error unless some underlying
// transport is misbehaving.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrAddrFiltered instead.
var ErrAddrFiltered = swarm.ErrAddrFiltered

// ErrDialTimeout is returned when one a dial times out due to the global timeout
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrDialTimeout instead.
var ErrDialTimeout = swarm.ErrDialTimeout

// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.Option instead.
type Option = swarm.Option

// WithConnectionGater sets a connection gater
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.WithConnectionGater instead.
func WithConnectionGater(gater connmgr.ConnectionGater) Option {
	return swarm.WithConnectionGater(gater)
}

// WithMetrics sets a metrics reporter
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.WithMetrics instead.
func WithMetrics(reporter metrics.Reporter) Option {
	return swarm.WithMetrics(reporter)
}

// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.WithDialTimeout instead.
func WithDialTimeout(t time.Duration) Option {
	return swarm.WithDialTimeout(t)
}

// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.WithDialTimeoutLocal instead.
func WithDialTimeoutLocal(t time.Duration) Option {
	return swarm.WithDialTimeoutLocal(t)
}

// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.WithResourceManager instead.
func WithResourceManager(m network.ResourceManager) Option {
	return swarm.WithResourceManager(m)
}

// Swarm is a connection muxer, allowing connections to other peers to
// be opened and closed, while still using the same Chan for all
// communication. The Chan sends/receives Messages, which note the
// destination or source Peer.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.Swarm instead.
type Swarm = swarm.Swarm

// NewSwarm constructs a Swarm.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.NewSwarm instead.
func NewSwarm(local peer.ID, peers peerstore.Peerstore, opts ...Option) (*Swarm, error) {
	return swarm.NewSwarm(local, peers, opts...)
}
