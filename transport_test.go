package swarm_test

import (
	"context"
	"testing"

	swarm "github.com/libp2p/go-libp2p-swarm"
	swarmt "github.com/libp2p/go-libp2p-swarm/testing"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/stretchr/testify/require"
)

type dummyTransport struct {
	protocols []ma.Protocol
	proxy     bool
	closed    bool
}

func (dt *dummyTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	panic("unimplemented")
}

func (dt *dummyTransport) CanDial(addr ma.Multiaddr) bool {
	panic("unimplemented")
}

func (dt *dummyTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	panic("unimplemented")
}

func (dt *dummyTransport) Proxy() bool {
	return dt.proxy
}

func (dt *dummyTransport) Protocols() []ma.Protocol {
	return dt.protocols
}
func (dt *dummyTransport) Close() error {
	dt.closed = true
	return nil
}

func TestUselessTransport(t *testing.T) {
	s := swarmt.GenSwarm(t)
	require.Error(t, s.AddTransport(new(dummyTransport)), "adding a transport that supports no protocols should have failed")
}

func TestTransportClose(t *testing.T) {
	s := swarmt.GenSwarm(t)
	tpt := &dummyTransport{protocols: []ma.Protocol{ma.ProtocolWithCode(ma.P_TCP)}}
	require.NoError(t, s.AddTransport(tpt))
	_ = s.Close()
	if !tpt.closed {
		t.Fatal("expected transport to be closed")
	}
}

func TestTransportAfterClose(t *testing.T) {
	s := swarmt.GenSwarm(t)
	s.Close()

	tpt := &dummyTransport{protocols: []ma.Protocol{ma.ProtocolWithCode(ma.P_TCP)}}
	if err := s.AddTransport(tpt); err != swarm.ErrSwarmClosed {
		t.Fatal("expected swarm closed error, got: ", err)
	}
}
