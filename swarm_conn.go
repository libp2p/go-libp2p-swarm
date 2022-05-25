package swarm

import (
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
)

// ErrConnClosed is returned when operating on a closed connection.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.ErrConnClosed instead.
var ErrConnClosed = swarm.ErrConnClosed

// Conn is the connection type used by swarm. In general, you won't use this
// type directly.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm.Conn instead.
type Conn = swarm.Conn
