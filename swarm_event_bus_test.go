package swarm_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	. "github.com/libp2p/go-libp2p-swarm"
	"github.com/stretchr/testify/require"
)

func TestEventBus(t *testing.T) {
	const swarmSize = 5

	ctx := context.Background()
	swarms := makeSwarms(ctx, t, swarmSize)
	defer func() {
		for _, s := range swarms {
			s.Close()
		}
	}()

	timeout := 5 * time.Second

	// subscribe to event bus for peer connections
	connSubs := make([]event.Subscription, len(swarms))
	// subscribe to event bus for stream notifs
	streamSubs := make([]event.Subscription, len(swarms))

	var err error
	for i, swarm := range swarms {
		connSubs[i], err = swarm.EventBus().Subscribe(&network.EvtPeerConnectionStateChange{})
		require.NoError(t, err)
		streamSubs[i], err = swarm.EventBus().Subscribe(&network.EvtStreamStateChange{})
		require.NoError(t, err)
		defer connSubs[i].Close()
		defer streamSubs[i].Close()
	}

	connectSwarms(t, ctx, swarms)

	<-time.After(time.Millisecond)
	// should've gotten 5 by now.

	// test everyone got the correct connection opened calls
	for i, s := range swarms {
		sub := connSubs[i]
		notifs := make(map[peer.ID][]network.Conn)
		for j, s2 := range swarms {
			if i == j {
				continue
			}

			// this feels a little sketchy, but its probably okay
			for len(s.ConnsToPeer(s2.LocalPeer())) != len(notifs[s2.LocalPeer()]) {
				select {
				case o := <-sub.Out():
					evt := o.(network.EvtPeerConnectionStateChange)
					require.Equal(t, network.Connected, evt.NewState)
					c := evt.Connection
					nfp := notifs[c.RemotePeer()]
					notifs[c.RemotePeer()] = append(nfp, c)
				case <-time.After(timeout):
					t.Fatal("timeout")
				}
			}
		}

		for p, cons := range notifs {
			expect := s.ConnsToPeer(p)
			if len(expect) != len(cons) {
				t.Fatal("got different number of connections")
			}

			for _, c := range cons {
				var found bool
				for _, c2 := range expect {
					if c == c2 {
						found = true
						break
					}
				}

				if !found {
					t.Fatal("connection not found!")
				}
			}
		}
	}

	complement := func(c network.Conn, isConn bool) (*Swarm, event.Subscription, *Conn) {
		var sub event.Subscription
		for i, s := range swarms {
			if isConn {
				sub = connSubs[i]
			} else {
				sub = streamSubs[i]
			}
			for _, c2 := range s.Conns() {
				if c.LocalMultiaddr().Equal(c2.RemoteMultiaddr()) &&
					c2.LocalMultiaddr().Equal(c.RemoteMultiaddr()) {

					return s, sub, c2.(*Conn)
				}
			}
		}
		t.Fatal("complementary conn not found", c)
		return nil, nil, nil
	}

	testOCStream := func(sub event.Subscription, s network.Stream) {
		var s2 network.Stream
		select {
		case o := <-sub.Out():
			evt := o.(network.EvtStreamStateChange)
			require.Equal(t, network.Connected, evt.NewState)
			s2 = evt.Stream
			t.Log("got notif for opened stream")
		case <-time.After(timeout):
			t.Fatal("timeout")
		}
		if s != s2 {
			t.Fatal("got incorrect stream", s.Conn(), s2.Conn())
		}

		select {
		case o := <-sub.Out():
			evt := o.(network.EvtStreamStateChange)
			require.Equal(t, network.NotConnected, evt.NewState)
			s2 = evt.Stream
			t.Log("got notif for closed stream")
		case <-time.After(timeout):
			t.Fatal("timeout")
		}
		if s != s2 {
			t.Fatal("got incorrect stream", s.Conn(), s2.Conn())
		}
	}

	streams := make(chan network.Stream)
	for _, s := range swarms {
		s.SetStreamHandler(func(s network.Stream) {
			streams <- s
			s.Reset()
		})
	}

	// open a streams in each conn
	for i, s := range swarms {
		for _, c := range s.Conns() {
			_, n2, _ := complement(c, false)

			st1, err := c.NewStream()
			if err != nil {
				t.Error(err)
			} else {
				st1.Write([]byte("hello"))
				st1.Reset()
				testOCStream(streamSubs[i], st1)
				st2 := <-streams
				testOCStream(n2, st2)
			}
		}
	}

	// close conns
	for i, s := range swarms {
		n := connSubs[i]
		for _, c := range s.Conns() {
			_, n2, c2 := complement(c, true)
			c.Close()
			c2.Close()

			var c3, c4 network.Conn
			select {
			case o := <-n.Out():
				evt := o.(network.EvtPeerConnectionStateChange)
				require.Equal(t, network.NotConnected, evt.NewState)
				c3 = evt.Connection
			case <-time.After(timeout):
				t.Fatal("timeout")
			}
			if c != c3 {
				t.Fatal("got incorrect conn", c, c3)
			}

			select {
			case o := <-n2.Out():
				evt := o.(network.EvtPeerConnectionStateChange)
				require.Equal(t, network.NotConnected, evt.NewState)
				c4 = evt.Connection
			case <-time.After(timeout):
				t.Fatal("timeout")
			}
			if c2 != c4 {
				t.Fatal("got incorrect conn", c, c2)
			}
		}
	}
}
