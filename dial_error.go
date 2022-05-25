package swarm

import (
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
)

// DialError is the error type returned when dialing.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.DialError instead.
type DialError = swarm.DialError

// TransportError is the error returned when dialing a specific address.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.TransportError instead.
type TransportError = swarm.TransportError
