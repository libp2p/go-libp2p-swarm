package swarm_test

import (
	"context"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/peerstore"
	swarmt "github.com/libp2p/go-libp2p-swarm/testing"
	"github.com/pkg/errors"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/network"
	. "github.com/libp2p/go-libp2p-swarm"
	"github.com/stretchr/testify/require"
)

func TestNotifieeEventbusSimple(t *testing.T) {
	ctx := context.Background()
	s1 := swarmt.GenSwarm(t, ctx)
	defer s1.Close()
	s2 := swarmt.GenSwarm(t, ctx)
	defer s2.Close()

	// subscribe for notifications on s1
	s, err := s1.EventBus().Subscribe(&event.EvtPeerConnectednessChanged{})
	defer s.Close()
	require.NoError(t, err)

	// connect to s1 to s2 so we get the first notificaion
	connectSwarms(t, ctx, []*Swarm{s1, s2})
	select {
	case e := <-s.Out():
		evt, ok := e.(event.EvtPeerConnectednessChanged)
		require.True(t, ok)
		require.Equal(t, network.Connected, evt.Connectedness)
		require.Equal(t, s2.LocalPeer(), evt.Peer)
	case <-time.After(1 * time.Second):
		t.Fatal("did not get notification")
	}

	// disconnect so we get a notification
	require.NoError(t, s1.ClosePeer(s2.LocalPeer()))
	select {
	case e := <-s.Out():
		evt, ok := e.(event.EvtPeerConnectednessChanged)
		require.True(t, ok)
		require.Equal(t, network.NotConnected, evt.Connectedness)
		require.Equal(t, s2.LocalPeer(), evt.Peer)
	case <-time.After(1 * time.Second):
		t.Fatal("did not get disconnect notification")
	}
}

func TestNotifieeEventbusConcurrent(t *testing.T) {
	ctx := context.Background()
	s1 := swarmt.GenSwarm(t, ctx)
	defer s1.Close()
	s2 := swarmt.GenSwarm(t, ctx)
	defer s2.Close()

	// subscribe for notifications on s1
	sub1, err := s1.EventBus().Subscribe(&event.EvtPeerConnectednessChanged{}, eventbus.BufSize(200))
	defer sub1.Close()
	require.NoError(t, err)

	// subscribe for notifications on s2
	sub2, err := s2.EventBus().Subscribe(&event.EvtPeerConnectednessChanged{}, eventbus.BufSize(200))
	defer sub2.Close()
	require.NoError(t, err)

	// create a connection between s1 & s2
	s1.Peerstore().AddAddrs(s2.LocalPeer(), s2.ListenAddresses(), peerstore.PermanentAddrTTL)
	s2.Peerstore().AddAddrs(s1.LocalPeer(), s1.ListenAddresses(), peerstore.PermanentAddrTTL)
	c, connErr := s1.DialPeer(ctx, s2.LocalPeer())
	require.NoError(t, connErr)
	require.NotNil(t, c)

	// ensure both get connected event
	select {
	case e := <-sub1.Out():
		evt, ok := e.(event.EvtPeerConnectednessChanged)
		require.True(t, ok)
		require.Equal(t, network.Connected, evt.Connectedness)
		require.Equal(t, s2.LocalPeer(), evt.Peer)
	case <-time.After(1 * time.Second):
		t.Fatal("did not get connected event")
	}

	select {
	case e := <-sub2.Out():
		evt, ok := e.(event.EvtPeerConnectednessChanged)
		require.True(t, ok)
		require.Equal(t, network.Connected, evt.Connectedness)
		require.Equal(t, s1.LocalPeer(), evt.Peer)
	case <-time.After(1 * time.Second):
		t.Fatal("did not get connected event")
	}

	// now make simultaneous disconnect<->connect from both sides
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			s1.ClosePeer(s2.LocalPeer())
			wg.Done()
		}()

		go func() {
			_, connErr = s2.DialPeer(ctx, s1.LocalPeer())
			connErr = errors.WithMessage(connErr, "s1 failed to dial to s2")
			wg.Done()
		}()
	}
	wg.Wait()

	require.NoError(t, connErr)

	// ENSURE FINAL STATE IS CORRECT ON S1
	var finalState *event.EvtPeerConnectednessChanged
LOOP:
	for {
		select {
		case e := <-sub1.Out():
			evt, ok := e.(event.EvtPeerConnectednessChanged)
			require.True(t, ok)
			if finalState != nil {
				require.NotEqual(t, finalState.Connectedness, evt.Connectedness)
			}
			finalState = &evt
		default:
			// since events are emitted before the dial function returns, this means there are no more events
			break LOOP
		}
	}

	require.NotNil(t, finalState)
	require.Equal(t, s2.LocalPeer(), finalState.Peer)
	require.Equal(t, s1.Connectedness(s2.LocalPeer()), finalState.Connectedness)

	finalState = nil
	// ENSURE FINAL STATE IS CORRECT ON S2
LOOP2:
	for {
		select {
		case e := <-sub2.Out():
			evt, ok := e.(event.EvtPeerConnectednessChanged)
			require.True(t, ok)
			if finalState != nil {
				require.NotEqual(t, finalState.Connectedness, evt.Connectedness)
			}
			finalState = &evt
		default:
			break LOOP2
		}
	}

	require.NotNil(t, finalState)
	require.Equal(t, s1.LocalPeer(), finalState.Peer)
	require.Equal(t, s2.Connectedness(s1.LocalPeer()), finalState.Connectedness)
}
