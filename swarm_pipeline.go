package swarm

import (
	addrutil "github.com/libp2p/go-addr-util"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/transport"
	dial "github.com/libp2p/go-libp2p-swarm/dial"
	ma "github.com/multiformats/go-multiaddr"
)

// NewDefaultPipeline returns a Pipeline suitable for simple host constructions. It is composed of:
//
//  - Preparer: a preparer sequence (PreparerSeq), made of:
//      1. validator: verifies that the peer ID is well formed, and that we're not dialing to ourselves.
//      2. timeout: sets the request timeout.
//      3. dedup: checks if there's an dial to the peer already in progress, and awaits its completion,
//         copying the result from it as a return value.
//      3. backoff: circuit-breaker that prevents faulty peers from being dialed again within a time window.
//  - AddressResolver: a simple address resolver that fetches addresses from the peerstore, performing no
//    dynamic discovery.
//  - Planner: an immediate planner that emits dial jobs for all addresses as they appear.
//  - Throttler: a throttler that limits concurrent dials per peer, as well as by file descriptor usage.
//  - Executor: a simple executor that launches a goroutine per dial job.
func (s *Swarm) NewDefaultPipeline() *dial.Pipeline {
	p := dial.NewPipeline(s.ctx, s, func(tc transport.CapableConn) (conn network.Conn, e error) {
		return s.addConn(tc, network.DirOutbound)
	})

	// preparers.
	seq := new(dial.PreparerSeq)
	seq.AddLast("validator", dial.NewValidator(s.LocalPeer()))
	seq.AddLast("timeout", dial.NewRequestTimeout())
	seq.AddLast("dedup", dial.NewDedup())
	seq.AddLast("backoff", dial.NewBackoff(dial.DefaultBackoffConfig()))
	_ = p.SetPreparer(seq)

	// dial address filters.
	var filters = dial.DefaultAddrFilters(s)

	// do we have a transport for dialing this address?
	filters = append(filters, func(addr ma.Multiaddr) bool {
		t := s.TransportForDialing(addr)
		return t != nil && t.CanDial(addr)
	})

	// is the address blocked?
	filters = append(filters, (dial.AddrFilterFn)(addrutil.FilterNeg(s.Filters.AddrBlocked)))

	// address resolver.
	_ = p.SetAddressResolver(dial.NewPeerstoreAddressResolver(s, filters...))

	// throttler.
	_ = p.SetThrottler(dial.NewThrottler())

	// planner.
	_ = p.SetPlanner(dial.NewImmediatePlanner())

	// executor.
	_ = p.SetExecutor(dial.NewExecutor(s.TransportForDialing, dial.SetJobTimeout))

	return p
}
