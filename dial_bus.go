package swarm

import (
	"context"
	"fmt"
	"sync"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"
)

// DialBus manage dialing.
type dialBus struct {
	p peer.ID
	s *Swarm

	// Used to cancel if no more dial are requested to this peer.
	wanting struct {
		sync.Mutex
		refcount uint
	}

	c struct {
		sync.RWMutex
		// Closed if c != nil.
		available chan struct{}
		// The current best connection avaible.
		conn *Conn
	}

	// Used for dials to cancel other if they are better.
	dials struct {
		sync.Mutex
		d *dialJob
	}

	err struct {
		sync.RWMutex
		// If available is closed means no more new dial are to expect.
		available chan struct{}
		// Value of the error.
		err error
	}
}

func (s *Swarm) newDialBus(p peer.ID) *dialBus {
	db := &dialBus{p: p, s: s}
	db.c.available = make(chan struct{})
	db.err.available = make(chan struct{})
	return db
}

func (s *Swarm) getOrCreateDialBus(p peer.ID) *dialBus {
	s.conns.RLock()
	if db := s.conns.m[p]; db != nil {
		s.conns.RUnlock()
		return db
	}
	s.conns.RUnlock()
	s.conns.Lock()
	db := s.newDialBus(p)
	s.conns.m[p] = db
	s.conns.Unlock()
	return db
}

var ErrDialCanceled = fmt.Errorf("All dial context were canceled.")

// doDial start a dialling operation.
func (d *dialBus) doDial(ctx context.Context) error {
	// TODO: Emit events
	s := d.s
	if d.p == s.local {
		return &DialError{Peer: d.p, Cause: ErrDialToSelf}
	}

	sk := s.peers.PrivKey(s.local)
	if sk == nil {
		// fine for sk to be nil, just log.
		log.Debug("Dial not given PrivateKey, so WILL NOT SECURE conn.")
	}

	peerAddrs := s.peers.Addrs(d.p)
	if len(peerAddrs) == 0 {
		return &DialError{Peer: d.p, Cause: ErrNoAddresses}
	}
	goodAddrs := s.filterKnownUndialables(peerAddrs)
	if len(goodAddrs) == 0 {
		return &DialError{Peer: d.p, Cause: ErrNoGoodAddresses}
	}

	d.dials.Lock()
	d.c.RLock()
	var currentQuality uint32
	if d.c.conn != nil {
		currentQuality = d.c.conn.conn.Quality()
	}
	// False if an other is already on the way or if we start one.
	var isNotDialling bool = d.dials.d == nil
AddrIterator:
	for _, addr := range goodAddrs {
		// Iterate over the linked list
		current := d.dials.d
		for current != nil {
			// Check if we are already dialling this address.
			if current.raddr == addr {
				continue AddrIterator
			}
			current = current.next
		}
		// If not start a dial to this address.
		ctxDial, cancel := context.WithCancel(context.Background())
		tpt := s.TransportForDialing(addr)
		if tpt == nil {
			continue
		}
		score, err := tpt.Score(addr, d.p)
		if err != nil || (d.c.conn != nil && score.Quality >= currentQuality) {
			continue
		}
		current.next = &dialJob{
			cancel:  cancel,
			raddr:   addr,
			quality: score.Quality,
		}
		isNotDialling = false
		// TODO: Implement fd limiting.
		go d.watchDial(ctxDial, current.next, tpt)
	}
	d.c.RUnlock()
	d.dials.Unlock()

	if isNotDialling {
		return &DialError{Peer: d.p, Cause: fmt.Errorf("Can't connect to %s, no dial started.", d.p.Pretty())}
	}

	// Start a context manager, this will monitor the status of the dial and
	// respond to the ctx.
	go func() {
		select {
		// Check if we get a connection.
		case <-d.c.available:
			// Great nothing to do.
		// Check if all dials were bad.
		case <-d.err.available:
			// Sad but still not our problem.
		// Finaly check if we don't want of this anymore.
		case <-ctx.Done():
			d.wanting.Lock()
			defer d.wanting.Unlock()
			d.wanting.refcount--
			// Checking if we were the last wanting this dial.
			if d.wanting.refcount == 0 {
				// If canceling all dial.
				// Aquire all locks to ensure a complete stop of the dialBus.
				d.dials.Lock()
				d.c.Lock()
				defer d.c.Unlock()
				d.err.Lock()
				defer d.err.Unlock()
				// Iterate over the linked list
				current := d.dials.d
				// TODO: move teardown logic to an external function
				for current != nil {
					// cancel each dial.
					current.cancel()
					current = current.next
				}
				d.dials.Unlock()
				if d.c.conn != nil {
					d.c.conn.Close()
				}
				// Safely close.
				select {
				case <-d.err.available:
				default:
					d.err.err = ErrDialCanceled
					close(d.err.available)
				}
			}
		}
	}()
	return nil
}

func (d *dialBus) watchDial(ctx context.Context, di *dialJob, tpt transport.QTransport) {
	var endingBad = true
	defer func() {
		// we have finish dialling, remove us from the job list.
		d.dials.Lock()
		defer d.dials.Unlock()
		// Start iterating
		current := d.dials.d
		var past *dialJob = nil
		for current != nil {
			if current == di {
				if past == nil {
					d.dials.d = current.next
				} else {
					past.next = current.next
				}
			}
			past, current = current, current.next
		}
		if endingBad {
			// Checking if we were the last dial.
			if d.dials.d == nil {
				// If raising an error.
				d.err.Lock()
				defer d.err.Unlock()
				// Safely close.
				select {
				case <-d.err.available:
				default:
					d.err.err = ErrAllDialsFailed
					close(d.err.available)
				}
			}
		}
	}()
	conn, err := tpt.Dial(ctx, di.raddr, d.p)
	if err != nil {
		log.Error(fmt.Errorf("Error dialing %s with transport %T: %s", d.p, tpt, err))
		return
	}
	// Check if we should die (e.g. a bad implemented transport not respecting the
	// context).
	select {
	case <-ctx.Done():
		conn.Close()
		return
	default:
	}
	// Trust the transport? Yeah... right.
	if conn.RemotePeer() != d.p {
		log.Error(fmt.Errorf("BUG in transport %T: tried to dial %s, dialed %s", tpt, d.p, conn.RemotePeer()))
		conn.Close()
		return
	}
	// Upgrading
	err = d.s.addConn(conn, network.DirOutbound)
	if err != nil {
		conn.Close()
		log.Error(fmt.Errorf("Error upgrading to network.Conn %s with transport %T: %s", d.p, tpt, err))
		return
	}
	endingBad = false
}

func (d *dialBus) watchForConn(ctx context.Context) (*Conn, error) {
	// Wait for a response.
	select {
	// First try to get the conn.
	case <-d.c.available:
		d.c.RLock()
		defer d.c.RUnlock()
		return d.c.conn, nil
	// Else try to get the error message.
	case <-d.err.available:
		d.err.RLock()
		defer d.err.RUnlock()
		return nil, d.err.err
	// And finaly verify if connection is still wanted.
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// dialJob is a linked list of dial item, each dial have one.
// Its a linked list to have light fast item deletion.
type dialJob struct {
	quality uint32
	cancel  func()
	raddr   ma.Multiaddr
	next    *dialJob
}
