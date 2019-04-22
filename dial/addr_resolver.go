package dial

import (
	addrutil "github.com/libp2p/go-addr-util"
	inet "github.com/libp2p/go-libp2p-net"
	ma "github.com/multiformats/go-multiaddr"
)

type AddrFilterFn = func(addr ma.Multiaddr) bool

var defaultFilters = struct {
	preventSelfDial          func(network inet.Network) AddrFilterFn
	preventIPv6LinkLocalDial func(network inet.Network) AddrFilterFn
}{
	preventSelfDial: func(net inet.Network) AddrFilterFn {
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
	},

	preventIPv6LinkLocalDial: func(_ inet.Network) AddrFilterFn {
		return addrutil.AddrOverNonLocalIP
	},
}

type pstoreAddrResolver struct {
	network inet.Network
	filters []AddrFilterFn
}

var _ AddressResolver = (*pstoreAddrResolver)(nil)

func NewPeerstoreAddressResolver(network inet.Network, useDefaultFilters bool, filters ...AddrFilterFn) AddressResolver {
	var fs []AddrFilterFn
	if useDefaultFilters {
		fs = append(fs,
			defaultFilters.preventSelfDial(network),
			defaultFilters.preventIPv6LinkLocalDial(network),
		)
	}
	fs = append(fs, filters...)
	return &pstoreAddrResolver{network: network, filters: fs}
}

func (par *pstoreAddrResolver) Resolve(req *Request) (known []ma.Multiaddr, more <-chan []ma.Multiaddr, err error) {
	known = par.network.Peerstore().Addrs(req.PeerID())
	if len(known) == 0 {
		return nil, nil, nil
	}

	known = addrutil.FilterAddrs(known, par.filters...)
	return known, nil, nil
}
