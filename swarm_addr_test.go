package swarm_test

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/test"

	ma "github.com/multiformats/go-multiaddr"

	swarmt "github.com/libp2p/go-libp2p-swarm/testing"
)

func TestDialBadAddrs(t *testing.T) {

	m := func(s string) ma.Multiaddr {
		maddr, err := ma.NewMultiaddr(s)
		if err != nil {
			t.Fatal(err)
		}
		return maddr
	}

	ctx := context.Background()
	s := makeSwarms(ctx, t, 1)[0]

	test := func(a ma.Multiaddr) {
		p := test.RandPeerIDFatal(t)
		s.Peerstore().AddAddr(p, a, peerstore.PermanentAddrTTL)
		if _, err := s.DialPeer(ctx, p); err == nil {
			t.Errorf("swarm should not dial: %s", p)
		}
	}

	test(m("/ip6/fe80::1"))                // link local
	test(m("/ip6/fe80::100"))              // link local
	test(m("/ip4/127.0.0.1/udp/1234/utp")) // utp
}

func TestAddrRace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := makeSwarms(ctx, t, 1)[0]
	defer s.Close()

	a1, err := s.InterfaceListenAddresses()
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.InterfaceListenAddresses()
	if err != nil {
		t.Fatal(err)
	}

	if len(a1) > 0 && len(a2) > 0 && &a1[0] == &a2[0] {
		t.Fatal("got the exact same address set twice; this could lead to data races")
	}
}

func TestAddressesWithoutListening(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := swarmt.GenSwarm(t, ctx, swarmt.OptDialOnly)

	a1, err := s.InterfaceListenAddresses()
	if err != nil {
		t.Fatal(err)
	}
	if len(a1) != 0 {
		t.Fatalf("expected to be listening on no addresses, was listening on %d", len(a1))
	}
}
