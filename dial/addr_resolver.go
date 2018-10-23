package dial

import (
	addrutil "github.com/libp2p/go-addr-util"
	ma "github.com/multiformats/go-multiaddr"
)

type AddrResolver struct {
	sFilters []func(ma.Multiaddr) bool
	dFilters []func(req *Request) func(ma.Multiaddr) bool
}

func DefaultStaticFilters() []func(ma.Multiaddr) bool {
	return []func(ma.Multiaddr) bool{
		addrutil.AddrOverNonLocalIP,
	}
}

func DefaultDynamicFilters() []func(req *Request) func(ma.Multiaddr) bool {
	excludeOurAddrs := func(req *Request) func(ma.Multiaddr) bool {
		lisAddrs, _ := req.net.InterfaceListenAddresses()
		var ourAddrs []ma.Multiaddr
		for _, addr := range lisAddrs {
			protos := addr.Protocols()
			if len(protos) == 2 && (protos[0].Code == ma.P_IP4 || protos[0].Code == ma.P_IP6) {
				// we're only sure about filtering out /ip4 and /ip6 addresses, so far
				ourAddrs = append(ourAddrs, addr)
			}
		}
		return addrutil.SubtractFilter(ourAddrs...)
	}

	return []func(req *Request) func(ma.Multiaddr) bool{
		excludeOurAddrs,
	}
}

type AddrFilterFactory func(req *Request) []func(ma.Multiaddr) bool

var _ RequestPreparer = (*validator)(nil)

func NewAddrResolver(staticFilters []func(ma.Multiaddr) bool, dynamicFilters []func(req *Request) func(ma.Multiaddr) bool) RequestPreparer {
	return &AddrResolver{
		sFilters: staticFilters,
		dFilters: dynamicFilters,
	}
}

func (m *AddrResolver) Prepare(req *Request) {
	req.addrs = req.net.Peerstore().Addrs(req.id)
	if len(req.addrs) == 0 {
		return
	}

	// apply the static filters.
	req.addrs = addrutil.FilterAddrs(req.addrs, m.sFilters...)
	if len(m.dFilters) == 0 {
		return
	}

	// apply the dynamic filters.
	var dFilters = make([]func(multiaddr ma.Multiaddr) bool, 0, len(m.dFilters))
	for _, df := range m.dFilters {
		dFilters = append(dFilters, df(req))
	}
	req.addrs = addrutil.FilterAddrs(req.addrs, dFilters...)
}
