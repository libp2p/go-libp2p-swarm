package dial

import (
	addrutil "github.com/libp2p/go-addr-util"
	inet "github.com/libp2p/go-libp2p-net"
	ma "github.com/multiformats/go-multiaddr"
)

type AddrFilterFactory func(net inet.Network) AddrFilterFn
type AddrFilterFn func(addr ma.Multiaddr) bool

type addrResolver struct {
	filters []AddrFilterFactory
}

var excludeLinkLocal = func(_ inet.Network) AddrFilterFn {
	return func(addr ma.Multiaddr) bool {
		return addrutil.AddrOverNonLocalIP(addr)
	}
}

var excludeOwn = func(net inet.Network) AddrFilterFn {
	lisAddrs, _ := net.InterfaceListenAddresses()
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

var defaultFilters = []AddrFilterFactory{
	excludeLinkLocal,
	excludeOwn,
}

var _ AddressResolver = (*addrResolver)(nil)

func NewAddrResolver(useDefaultFilters bool, filters ...AddrFilterFactory) AddressResolver {
	var f []AddrFilterFactory
	if useDefaultFilters {
		f = append(f, defaultFilters...)
	}
	f = append(f, filters...)
	return &addrResolver{filters: f}
}

func (ar *addrResolver) Resolve(req *Request) (more <-chan []ma.Multiaddr, err error) {
	addrs := req.net.Peerstore().Addrs(req.id)
	if len(addrs) == 0 {
		return nil, nil
	}

	filters := make([]func(multiaddr ma.Multiaddr) bool, 0, len(ar.filters))
	for _, f := range ar.filters {
		filters = append(filters, f(req.net))
	}

	req.addrs = addrutil.FilterAddrs(addrs, filters...)
	return nil, nil
}
