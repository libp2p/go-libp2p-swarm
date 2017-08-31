package swarm

import (
	"context"
	"fmt"

	conn "github.com/libp2p/go-libp2p-conn"
	iconn "github.com/libp2p/go-libp2p-interface-conn"
	lgbl "github.com/libp2p/go-libp2p-loggables"
	mconn "github.com/libp2p/go-libp2p-metrics/conn"
	inet "github.com/libp2p/go-libp2p-net"
	tpt "github.com/libp2p/go-libp2p-transport"
	ps "github.com/libp2p/go-peerstream"
	ma "github.com/multiformats/go-multiaddr"
)

func (s *Swarm) AddListenAddr(a ma.Multiaddr) error {
	transport := s.transportForAddr(a)
	if transport == nil {
		return fmt.Errorf("no transport for address: %s", a)
	}

	d, err := transport.Dialer(a, tpt.ReusePorts)
	if err != nil {
		return err
	}
	s.dialer.AddDialer(d)

	list, err := transport.Listen(a)
	if err != nil {
		return err
	}

	err = s.addListener(list)
	if err != nil {
		return err
	}

	return nil
}

// Open listeners and reuse-dialers for the given addresses
func (s *Swarm) setupInterfaces(addrs []ma.Multiaddr) error {
	errs := make([]error, len(addrs))
	var succeeded int
	for i, a := range addrs {
		if err := s.AddListenAddr(a); err != nil {
			errs[i] = err
		} else {
			succeeded++
		}
	}

	for i, e := range errs {
		if e != nil {
			log.Warningf("listen on %s failed: %s", addrs[i], errs[i])
		}
	}

	if succeeded == 0 && len(addrs) > 0 {
		return fmt.Errorf("failed to listen on any addresses: %s", errs)
	}

	return nil
}

func (s *Swarm) transportForAddr(a ma.Multiaddr) tpt.Transport {
	for _, t := range s.transports {
		if t.Matches(a) {
			return t
		}
	}

	return nil
}

func (s *Swarm) addListener(tptlist tpt.Listener) error {
	sk := s.peers.PrivKey(s.local)
	if sk == nil {
		// may be fine for sk to be nil, just log a warning.
		log.Warning("Listener not given PrivateKey, so WILL NOT SECURE conns.")
	}

	// this encrypts the connection
	var list iconn.Listener
	var err error
	list, err = conn.WrapTransportListenerWithProtector(s.Context(), tptlist, s.local, sk, s.streamMuxers, s.protec)
	if err != nil {
		return err
	}

	list.SetAddrFilters(s.Filters)

	if cw, ok := list.(conn.ListenerConnWrapper); ok && s.bwc != nil {
		cw.SetConnWrapper(func(c tpt.Conn) tpt.Conn {
			return mconn.WrapConn(s.bwc, c)
		})
	}

	return s.addConnListener(list)
}

func (s *Swarm) addConnListener(list iconn.Listener) error {
	// AddListener to the peerstream Listener. this will begin accepting connections
	// and streams!
	sl, err := s.swarm.AddListener(list)
	if err != nil {
		return err
	}
	log.Debugf("Swarm Listeners at %s", s.ListenAddresses())

	maddr := list.Multiaddr()

	// signal to our notifiees on successful conn.
	s.notifyAll(func(n inet.Notifiee) {
		n.Listen((*Network)(s), maddr)
	})

	// go consume peerstream's listen accept errors. note, these ARE errors.
	// they may be killing the listener, and if we get _any_ we should be
	// fixing this in our conn.Listener (to ignore them or handle them
	// differently.)
	go func(ctx context.Context, sl *ps.Listener) {
		// signal to our notifiees closing
		defer s.notifyAll(func(n inet.Notifiee) {
			n.ListenClose((*Network)(s), maddr)
		})

		for {
			select {
			case err, more := <-sl.AcceptErrors():
				if !more {
					return
				}
				log.Warningf("swarm listener accept error: %s", err)
			case <-ctx.Done():
				return
			}
		}
	}(s.Context(), sl)

	return nil
}

// connHandler is called by the StreamSwarm whenever a new connection is added
// here we configure it slightly. Note that this is sequential, so if anything
// will take a while do it in a goroutine.
// See https://godoc.org/github.com/jbenet/go-peerstream for more information
func (s *Swarm) connHandler(c *ps.Conn) *Conn {
	ctx := context.Background()
	// this context is for running the handshake, which -- when receiveing connections
	// -- we have no bound on beyond what the transport protocol bounds it at.
	// note that setup + the handshake are bounded by underlying io.
	// (i.e. if TCP or UDP disconnects (or the swarm closes), we're done.
	// Q: why not have a shorter handshake? think about an HTTP server on really slow conns.
	// as long as the conn is live (TCP says its online), it tries its best. we follow suit.)

	sc, err := s.newConnSetup(c)
	if err != nil {
		log.Debug(err)
		log.Event(ctx, "newConnHandlerDisconnect", lgbl.Conn(c.Conn()), lgbl.Error(err))
		c.Close() // boom. close it.
		return nil
	}

	// if a peer dials us, remove from dial backoff.
	s.backf.Clear(sc.RemotePeer())

	return sc
}
