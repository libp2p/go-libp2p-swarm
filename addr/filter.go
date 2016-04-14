package addrutil

import (
	ma "github.com/jbenet/go-multiaddr"
	mafmt "github.com/whyrusleeping/mafmt"
)

func SubtractFilter(addrs ...ma.Multiaddr) func(ma.Multiaddr) bool {
	addrmap := make(map[string]bool)
	for _, a := range addrs {
		addrmap[string(a.Bytes())] = true
	}

	return func(a ma.Multiaddr) bool {
		return !addrmap[string(a.Bytes())]
	}
}

func IsFDCostlyTransport(a ma.Multiaddr) bool {
	return mafmt.TCP.Matches(a)
}

func FilterNeg(f func(ma.Multiaddr) bool) func(ma.Multiaddr) bool {
	return func(a ma.Multiaddr) bool {
		return !f(a)
	}
}
