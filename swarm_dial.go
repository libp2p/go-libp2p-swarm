package swarm

import (
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
)

// Diagram of dial sync:
//
//   many callers of Dial()   synched w.  dials many addrs       results to callers
//  ----------------------\    dialsync    use earliest            /--------------
//  -----------------------\              |----------\           /----------------
//  ------------------------>------------<-------     >---------<-----------------
//  -----------------------|              \----x                 \----------------
//  ----------------------|                \-----x                \---------------
//                                         any may fail          if no addr at end
//                                                             retry dialAttempt x

var (
	// ErrDialBackoff is returned by the backoff code when a given peer has
	// been dialed too frequently
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrDialBackoff instead.
	ErrDialBackoff = swarm.ErrDialBackoff

	// ErrDialToSelf is returned if we attempt to dial our own peer
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrDialToSelf instead.
	ErrDialToSelf = swarm.ErrDialToSelf

	// ErrNoTransport is returned when we don't know a transport for the
	// given multiaddr.
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrNoTransport instead.
	ErrNoTransport = swarm.ErrNoTransport

	// ErrAllDialsFailed is returned when connecting to a peer has ultimately failed
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrAllDialsFailed instead.
	ErrAllDialsFailed = swarm.ErrAllDialsFailed

	// ErrNoAddresses is returned when we fail to find any addresses for a
	// peer we're trying to dial.
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrNoAddresses instead.
	ErrNoAddresses = swarm.ErrNoAddresses

	// ErrNoGoodAddresses is returned when we find addresses for a peer but
	// can't use any of them.
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrNoGoodAddresses instead.
	ErrNoGoodAddresses = swarm.ErrNoGoodAddresses

	// ErrGaterDisallowedConnection is returned when the gater prevents us from
	// forming a connection with a peer.
	// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrGaterDisallowedConnection instead.
	ErrGaterDisallowedConnection = swarm.ErrGaterDisallowedConnection
)

// DialAttempts governs how many times a goroutine will try to dial a given peer.
// Note: this is down to one, as we have _too many dials_ atm. To add back in,
// add loop back in Dial(.)
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.DialAttempts instead.
const DialAttempts = swarm.DialAttempts

// ConcurrentFdDials is the number of concurrent outbound dials over transports
// that consume file descriptors
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ConcurrentFdDials instead.
const ConcurrentFdDials = swarm.ConcurrentFdDials

// DefaultPerPeerRateLimit is the number of concurrent outbound dials to make
// per peer
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.DefaultPerPeerRateLimit instead.
const DefaultPerPeerRateLimit = swarm.DefaultPerPeerRateLimit

// DialBackoff is a type for tracking peer dial backoffs.
//
// * It's safe to use its zero value.
// * It's thread-safe.
// * It's *not* safe to move this type after using.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.DialBackoff instead.
type DialBackoff = swarm.DialBackoff

// BackoffBase is the base amount of time to backoff (default: 5s).
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.BackoffBase instead.
var BackoffBase = swarm.BackoffBase

// BackoffCoef is the backoff coefficient (default: 1s).
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.BackoffCoef instead.
var BackoffCoef = swarm.BackoffCoef

// BackoffMax is the maximum backoff time (default: 5m).
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.BackoffMax instead.
var BackoffMax = swarm.BackoffMax
