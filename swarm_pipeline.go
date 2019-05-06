package swarm

import (
	addrutil "github.com/libp2p/go-addr-util"
	inet "github.com/libp2p/go-libp2p-net"
	dial "github.com/libp2p/go-libp2p-swarm/dial"
	transport "github.com/libp2p/go-libp2p-transport"

	ma "github.com/multiformats/go-multiaddr"
)

func (s *Swarm) NewDefaultPipeline() *dial.Pipeline {
	p := dial.NewPipeline(s.ctx, s, func(tc transport.Conn) (conn inet.Conn, e error) {
		return s.addConn(tc, inet.DirOutbound)
	})

	// preparers.
	seq := new(dial.PreparerSeq)
	seq.AddLast("validator", dial.NewValidator(s.LocalPeer()))
	seq.AddLast("timeout", dial.NewRequestTimeout())
	seq.AddLast("dedup", dial.NewDedup())
	seq.AddLast("backoff", dial.NewBackoff())
	p.SetPreparer(seq)

	// dial address filters.
	var filters []dial.AddrFilterFn

	// do we have a transport for dialing this address?
	filters = append(filters, func(addr ma.Multiaddr) bool {
		t := s.TransportForDialing(addr)
		return t != nil && t.CanDial(addr)
	})

	// is the address blocked?
	filters = append(filters, (dial.AddrFilterFn)(addrutil.FilterNeg(s.Filters.AddrBlocked)))

	// address resolver.
	p.SetAddressResolver(dial.NewPeerstoreAddressResolver(s, true, filters...))

	// throttler.
	p.SetThrottler(dial.NewDefaultThrottler())

	// planner.
	p.SetPlanner(dial.NewImmediatePlanner())

	// executor.
	p.SetExecutor(dial.NewExecutor(s.TransportForDialing, dial.SetJobTimeout))

	return p
}
