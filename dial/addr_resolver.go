package dial

import (
	addrutil "github.com/libp2p/go-addr-util"
	"github.com/libp2p/go-libp2p-core/network"
	ma "github.com/multiformats/go-multiaddr"
)

// AddrFilterFn is a predicate that validates if a multiaddr should be attempted or not.
type AddrFilterFn = func(addr ma.Multiaddr) bool

// defaultFilters contains factory methods to generate default filters based on a Network.
var defaultFilters = struct {
	preventSelfDial          func(network network.Network) AddrFilterFn
	preventIPv6LinkLocalDial func(network network.Network) AddrFilterFn
}{
	preventSelfDial: func(net network.Network) AddrFilterFn {
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

	preventIPv6LinkLocalDial: func(_ network.Network) AddrFilterFn {
		return addrutil.AddrOverNonLocalIP
	},
}

// DefaultAddrFilters returns the following address filters, recommended defaults:
//
//   1. preventSelfDial: aborts dial requests to ourselves.
//   2. preventIPv6LinkLocalDials: aborts dial requests to link local addresses.
func DefaultAddrFilters(network network.Network) (res []AddrFilterFn) {
	res = append(res,
		defaultFilters.preventSelfDial(network),
		defaultFilters.preventIPv6LinkLocalDial(network))
	return res
}

type pstoreAddrResolver struct {
	network network.Network
	filters []AddrFilterFn
}

var _ AddressResolver = (*pstoreAddrResolver)(nil)

// NewPeerstoreAddressResolver returns an AddressResolver that fetches known addresses from the peerstore,
// running no external discovery process.
func NewPeerstoreAddressResolver(network network.Network, filters ...AddrFilterFn) AddressResolver {
	return &pstoreAddrResolver{network: network, filters: filters}
}

func (par *pstoreAddrResolver) Resolve(req *Request) (known []ma.Multiaddr, more <-chan []ma.Multiaddr, err error) {
	known = par.network.Peerstore().Addrs(req.PeerID())
	if len(known) == 0 {
		req.Debugf("no addresses in peerstore")
		return nil, nil, nil
	}

	known = addrutil.FilterAddrs(known, par.filters...)
	req.Debugf("addresses after filtering: %v", known)
	return known, nil, nil
}
