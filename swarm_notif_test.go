package swarm_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	ma "github.com/multiformats/go-multiaddr"

	. "github.com/libp2p/go-libp2p-swarm"
)

func TestNotifications(t *testing.T) {
	const swarmSize = 5

	notifiees := make([]*netNotifiee, swarmSize)

	ctx := context.Background()
	swarms := makeSwarms(ctx, t, swarmSize)
	defer func() {
		for i, s := range swarms {
			select {
			case <-notifiees[i].listenClose:
				t.Error("should not have been closed")
			default:
			}
			err := s.Close()
			if err != nil {
				t.Error(err)
			}
			select {
			case <-notifiees[i].listenClose:
			default:
				t.Error("expected a listen close notification")
			}
		}
	}()

	timeout := 5 * time.Second

	// signup notifs
	for i, swarm := range swarms {
		n := newNetNotifiee(swarmSize)
		swarm.Notify(n)
		notifiees[i] = n
	}

	connectSwarms(t, ctx, swarms)

	<-time.After(time.Millisecond)
	// should've gotten 5 by now.

	// test everyone got the correct connection opened calls
	for i, s := range swarms {
		n := notifiees[i]
		notifs := make(map[peer.ID][]network.Conn)
		for j, s2 := range swarms {
			if i == j {
				continue
			}

			// this feels a little sketchy, but its probably okay
			for len(s.ConnsToPeer(s2.LocalPeer())) != len(notifs[s2.LocalPeer()]) {
				select {
				case c := <-n.connected:
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

	complement := func(c network.Conn) (*Swarm, *netNotifiee, *Conn) {
		for i, s := range swarms {
			for _, c2 := range s.Conns() {
				if c.LocalMultiaddr().Equal(c2.RemoteMultiaddr()) &&
					c2.LocalMultiaddr().Equal(c.RemoteMultiaddr()) {
					return s, notifiees[i], c2.(*Conn)
				}
			}
		}
		t.Fatal("complementary conn not found", c)
		return nil, nil, nil
	}

	testOCStream := func(n *netNotifiee, s network.Stream) {
		var s2 network.Stream
		select {
		case s2 = <-n.openedStream:
			t.Log("got notif for opened stream")
		case <-time.After(timeout):
			t.Fatal("timeout")
		}
		if s != s2 {
			t.Fatal("got incorrect stream", s.Conn(), s2.Conn())
		}

		select {
		case s2 = <-n.closedStream:
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
			_, n2, _ := complement(c)

			st1, err := c.NewStream(context.Background())
			if err != nil {
				t.Error(err)
			} else {
				st1.Write([]byte("hello"))
				st1.Reset()
				testOCStream(notifiees[i], st1)
				st2 := <-streams
				testOCStream(n2, st2)
			}
		}
	}

	// close conns
	for i, s := range swarms {
		n := notifiees[i]
		for _, c := range s.Conns() {
			_, n2, c2 := complement(c)
			c.Close()
			c2.Close()

			var c3, c4 network.Conn
			select {
			case c3 = <-n.disconnected:
			case <-time.After(timeout):
				t.Fatal("timeout")
			}
			if c != c3 {
				t.Fatal("got incorrect conn", c, c3)
			}

			select {
			case c4 = <-n2.disconnected:
			case <-time.After(timeout):
				t.Fatal("timeout")
			}
			if c2 != c4 {
				t.Fatal("got incorrect conn", c, c2)
			}
		}
	}
}

type netNotifiee struct {
	listen       chan ma.Multiaddr
	listenClose  chan ma.Multiaddr
	connected    chan network.Conn
	disconnected chan network.Conn
	openedStream chan network.Stream
	closedStream chan network.Stream
}

func newNetNotifiee(buffer int) *netNotifiee {
	return &netNotifiee{
		listen:       make(chan ma.Multiaddr, buffer),
		listenClose:  make(chan ma.Multiaddr, buffer),
		connected:    make(chan network.Conn, buffer),
		disconnected: make(chan network.Conn, buffer),
		openedStream: make(chan network.Stream, buffer),
		closedStream: make(chan network.Stream, buffer),
	}
}

func (nn *netNotifiee) Listen(n network.Network, a ma.Multiaddr) {
	nn.listen <- a
}
func (nn *netNotifiee) ListenClose(n network.Network, a ma.Multiaddr) {
	nn.listenClose <- a
}
func (nn *netNotifiee) Connected(n network.Network, v network.Conn) {
	nn.connected <- v
}
func (nn *netNotifiee) Disconnected(n network.Network, v network.Conn) {
	nn.disconnected <- v
}
func (nn *netNotifiee) OpenedStream(n network.Network, v network.Stream) {
	nn.openedStream <- v
}
func (nn *netNotifiee) ClosedStream(n network.Network, v network.Stream) {
	nn.closedStream <- v
}
