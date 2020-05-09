package swarm

import (
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

// BestConn supports user-defined connection selection algorithm
// BestConnFallback is BestConn's downgrade algorithm. When BestConn
// cann't select a connection, we downgrade using BestConnFallback to
// select the connection.
type BestConn interface {
	BestConn(peer.ID, []*Conn) *Conn
	BestConnFallback(peer.ID, []*Conn) *Conn
}

// If there is multiple good address can connect to the peer,
// We use this interface to select the best address to peer.
type BestDest interface {
	BestDestSelect(peer.ID, []ma.Multiaddr) []ma.Multiaddr
}
