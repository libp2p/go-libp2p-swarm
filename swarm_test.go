package swarm_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/control"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/test"

	. "github.com/libp2p/go-libp2p-swarm"
	. "github.com/libp2p/go-libp2p-swarm/testing"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

var log = logging.Logger("swarm_test")

func EchoStreamHandler(stream network.Stream) {
	go func() {
		defer stream.Close()

		// pull out the ipfs conn
		c := stream.Conn()
		log.Infof("%s ponging to %s", c.LocalPeer(), c.RemotePeer())

		buf := make([]byte, 4)

		for {
			if _, err := stream.Read(buf); err != nil {
				if err != io.EOF {
					log.Error("ping receive error:", err)
				}
				return
			}

			if !bytes.Equal(buf, []byte("ping")) {
				log.Errorf("ping receive error: ping != %s %v", buf, buf)
				return
			}

			if _, err := stream.Write([]byte("pong")); err != nil {
				log.Error("pond send error:", err)
				return
			}
		}
	}()
}

func makeDialOnlySwarm(ctx context.Context, t *testing.T) *Swarm {
	swarm := GenSwarm(t, ctx, OptDialOnly)
	swarm.SetStreamHandler(EchoStreamHandler)

	return swarm
}

func makeSwarms(ctx context.Context, t *testing.T, num int, opts ...Option) []*Swarm {
	swarms := make([]*Swarm, 0, num)

	for i := 0; i < num; i++ {
		swarm := GenSwarm(t, ctx, opts...)
		swarm.SetStreamHandler(EchoStreamHandler)
		swarms = append(swarms, swarm)
	}

	return swarms
}

func connectSwarms(t *testing.T, ctx context.Context, swarms []*Swarm) {

	var wg sync.WaitGroup
	connect := func(s *Swarm, dst peer.ID, addr ma.Multiaddr) {
		// TODO: make a DialAddr func.
		s.Peerstore().AddAddr(dst, addr, peerstore.PermanentAddrTTL)
		if _, err := s.DialPeer(ctx, dst); err != nil {
			t.Fatal("error swarm dialing to peer", err)
		}
		wg.Done()
	}

	log.Info("Connecting swarms simultaneously.")
	for i, s1 := range swarms {
		for _, s2 := range swarms[i+1:] {
			wg.Add(1)
			connect(s1, s2.LocalPeer(), s2.ListenAddresses()[0]) // try the first.
		}
	}
	wg.Wait()

	for _, s := range swarms {
		log.Infof("%s swarm routing table: %s", s.LocalPeer(), s.Peers())
	}
}

func SubtestSwarm(t *testing.T, SwarmNum int, MsgNum int) {
	// t.Skip("skipping for another test")

	ctx := context.Background()
	swarms := makeSwarms(ctx, t, SwarmNum, OptDisableReuseport)

	// connect everyone
	connectSwarms(t, ctx, swarms)

	// ping/pong
	for _, s1 := range swarms {
		log.Debugf("-------------------------------------------------------")
		log.Debugf("%s ping pong round", s1.LocalPeer())
		log.Debugf("-------------------------------------------------------")

		_, cancel := context.WithCancel(ctx)
		got := map[peer.ID]int{}
		errChan := make(chan error, MsgNum*len(swarms))
		streamChan := make(chan network.Stream, MsgNum)

		// send out "ping" x MsgNum to every peer
		go func() {
			defer close(streamChan)

			var wg sync.WaitGroup
			send := func(p peer.ID) {
				defer wg.Done()

				// first, one stream per peer (nice)
				stream, err := s1.NewStream(ctx, p)
				if err != nil {
					errChan <- err
					return
				}

				// send out ping!
				for k := 0; k < MsgNum; k++ { // with k messages
					msg := "ping"
					log.Debugf("%s %s %s (%d)", s1.LocalPeer(), msg, p, k)
					if _, err := stream.Write([]byte(msg)); err != nil {
						errChan <- err
						continue
					}
				}

				// read it later
				streamChan <- stream
			}

			for _, s2 := range swarms {
				if s2.LocalPeer() == s1.LocalPeer() {
					continue // dont send to self...
				}

				wg.Add(1)
				go send(s2.LocalPeer())
			}
			wg.Wait()
		}()

		// receive "pong" x MsgNum from every peer
		go func() {
			defer close(errChan)
			count := 0
			countShouldBe := MsgNum * (len(swarms) - 1)
			for stream := range streamChan { // one per peer
				defer stream.Close()

				// get peer on the other side
				p := stream.Conn().RemotePeer()

				// receive pings
				msgCount := 0
				msg := make([]byte, 4)
				for k := 0; k < MsgNum; k++ { // with k messages

					// read from the stream
					if _, err := stream.Read(msg); err != nil {
						errChan <- err
						continue
					}

					if string(msg) != "pong" {
						errChan <- fmt.Errorf("unexpected message: %s", msg)
						continue
					}

					log.Debugf("%s %s %s (%d)", s1.LocalPeer(), msg, p, k)
					msgCount++
				}

				got[p] = msgCount
				count += msgCount
			}

			if count != countShouldBe {
				errChan <- fmt.Errorf("count mismatch: %d != %d", count, countShouldBe)
			}
		}()

		// check any errors (blocks till consumer is done)
		for err := range errChan {
			if err != nil {
				t.Error(err.Error())
			}
		}

		log.Debugf("%s got pongs", s1.LocalPeer())
		if (len(swarms) - 1) != len(got) {
			t.Errorf("got (%d) less messages than sent (%d).", len(got), len(swarms))
		}

		for p, n := range got {
			if n != MsgNum {
				t.Error("peer did not get all msgs", p, n, "/", MsgNum)
			}
		}

		cancel()
		<-time.After(10 * time.Millisecond)
	}

	for _, s := range swarms {
		s.Close()
	}
}

func TestSwarm(t *testing.T) {
	// t.Skip("skipping for another test")
	t.Parallel()

	// msgs := 1000
	msgs := 100
	swarms := 5
	SubtestSwarm(t, swarms, msgs)
}

func TestBasicSwarm(t *testing.T) {
	// t.Skip("skipping for another test")
	t.Parallel()

	msgs := 1
	swarms := 2
	SubtestSwarm(t, swarms, msgs)
}

func TestConnHandler(t *testing.T) {
	// t.Skip("skipping for another test")
	t.Parallel()

	ctx := context.Background()
	swarms := makeSwarms(ctx, t, 5)

	gotconn := make(chan struct{}, 10)
	swarms[0].SetConnHandler(func(conn network.Conn) {
		gotconn <- struct{}{}
	})

	connectSwarms(t, ctx, swarms)

	<-time.After(time.Millisecond)
	// should've gotten 5 by now.

	swarms[0].SetConnHandler(nil)

	expect := 4
	for i := 0; i < expect; i++ {
		select {
		case <-time.After(time.Second):
			t.Fatal("failed to get connections")
		case <-gotconn:
		}
	}

	select {
	case <-gotconn:
		t.Fatalf("should have connected to %d swarms, got an extra.", expect)
	default:
	}
}

func TestConnectionGating(t *testing.T) {
	ctx := context.Background()
	tcs := map[string]struct {
		p1Gater func(gater *MockConnectionGater) *MockConnectionGater
		p2Gater func(gater *MockConnectionGater) *MockConnectionGater

		p1ConnectednessToP2 network.Connectedness
		p2ConnectednessToP1 network.Connectedness
		isP1OutboundErr     bool
	}{
		"no gating": {
			p1ConnectednessToP2: network.Connected,
			p2ConnectednessToP1: network.Connected,
			isP1OutboundErr:     false,
		},
		"p1 gates outbound peer dial": {
			p1Gater: func(c *MockConnectionGater) *MockConnectionGater {
				c.PeerDial = func(p peer.ID) bool { return false }
				return c
			},
			p1ConnectednessToP2: network.NotConnected,
			p2ConnectednessToP1: network.NotConnected,
			isP1OutboundErr:     true,
		},
		"p1 gates outbound addr dialing": {
			p1Gater: func(c *MockConnectionGater) *MockConnectionGater {
				c.Dial = func(p peer.ID, addr ma.Multiaddr) bool { return false }
				return c
			},
			p1ConnectednessToP2: network.NotConnected,
			p2ConnectednessToP1: network.NotConnected,
			isP1OutboundErr:     true,
		},
		"p2 gates inbound peer dial before securing": {
			p2Gater: func(c *MockConnectionGater) *MockConnectionGater {
				c.Accept = func(c network.ConnMultiaddrs) bool { return false }
				return c
			},
			p1ConnectednessToP2: network.NotConnected,
			p2ConnectednessToP1: network.NotConnected,
			isP1OutboundErr:     true,
		},
		"p2 gates inbound peer dial before multiplexing": {
			p1Gater: func(c *MockConnectionGater) *MockConnectionGater {
				c.Secured = func(network.Direction, peer.ID, network.ConnMultiaddrs) bool { return false }
				return c
			},
			p1ConnectednessToP2: network.NotConnected,
			p2ConnectednessToP1: network.NotConnected,
			isP1OutboundErr:     true,
		},
		"p2 gates inbound peer dial after upgrading": {
			p1Gater: func(c *MockConnectionGater) *MockConnectionGater {
				c.Upgraded = func(c network.Conn) (bool, control.DisconnectReason) { return false, 0 }
				return c
			},
			p1ConnectednessToP2: network.NotConnected,
			p2ConnectednessToP1: network.NotConnected,
			isP1OutboundErr:     true,
		},
		"p2 gates outbound dials": {
			p2Gater: func(c *MockConnectionGater) *MockConnectionGater {
				c.PeerDial = func(p peer.ID) bool { return false }
				return c
			},
			p1ConnectednessToP2: network.Connected,
			p2ConnectednessToP1: network.Connected,
			isP1OutboundErr:     false,
		},
	}

	for n, tc := range tcs {
		t.Run(n, func(t *testing.T) {
			p1Gater := DefaultMockConnectionGater()
			p2Gater := DefaultMockConnectionGater()
			if tc.p1Gater != nil {
				p1Gater = tc.p1Gater(p1Gater)
			}
			if tc.p2Gater != nil {
				p2Gater = tc.p2Gater(p2Gater)
			}

			sw1 := GenSwarm(t, ctx, OptConnGater(p1Gater))
			sw2 := GenSwarm(t, ctx, OptConnGater(p2Gater))

			p1 := sw1.LocalPeer()
			p2 := sw2.LocalPeer()
			sw1.Peerstore().AddAddr(p2, sw2.ListenAddresses()[0], peerstore.PermanentAddrTTL)
			// 1 -> 2
			_, err := sw1.DialPeer(ctx, p2)

			require.Equal(t, tc.isP1OutboundErr, err != nil, n)
			require.Equal(t, tc.p1ConnectednessToP2, sw1.Connectedness(p2), n)

			require.Eventually(t, func() bool {
				return tc.p2ConnectednessToP1 == sw2.Connectedness(p1)
			}, 2*time.Second, 100*time.Millisecond, n)
		})

	}
}

func TestIsFdConsuming(t *testing.T) {
	tcs := map[string]struct {
		addr          string
		isFdConsuming bool
	}{
		"tcp": {
			addr:          "/ip4/127.0.0.1/tcp/20",
			isFdConsuming: true,
		},
		"quic": {
			addr:          "/ip4/127.0.0.1/udp/0/quic",
			isFdConsuming: false,
		},
		"addr-without-registered-transport": {
			addr:          "/ip4/127.0.0.1/tcp/20/ws",
			isFdConsuming: true,
		},
		"relay-tcp": {
			addr:          fmt.Sprintf("/ip4/127.0.0.1/tcp/20/p2p-circuit/p2p/%s", test.RandPeerIDFatal(t)),
			isFdConsuming: true,
		},
		"relay-quic": {
			addr:          fmt.Sprintf("/ip4/127.0.0.1/udp/20/quic/p2p-circuit/p2p/%s", test.RandPeerIDFatal(t)),
			isFdConsuming: false,
		},
		"relay-without-serveraddr": {
			addr:          fmt.Sprintf("/p2p-circuit/p2p/%s", test.RandPeerIDFatal(t)),
			isFdConsuming: true,
		},
		"relay-without-registered-transport-server": {
			addr:          fmt.Sprintf("/ip4/127.0.0.1/tcp/20/ws/p2p-circuit/p2p/%s", test.RandPeerIDFatal(t)),
			isFdConsuming: true,
		},
	}

	ctx := context.Background()
	sw := GenSwarm(t, ctx)
	sk := sw.Peerstore().PrivKey(sw.LocalPeer())
	require.NotNil(t, sk)

	for name := range tcs {
		maddr, err := ma.NewMultiaddr(tcs[name].addr)
		require.NoError(t, err, name)
		require.Equal(t, tcs[name].isFdConsuming, sw.IsFdConsumingAddr(maddr), name)
	}
}

func TestNoDial(t *testing.T) {
	ctx := context.Background()
	swarms := makeSwarms(ctx, t, 2)

	_, err := swarms[0].NewStream(network.WithNoDial(ctx, "swarm test"), swarms[1].LocalPeer())
	if err != network.ErrNoConn {
		t.Fatal("should have failed with ErrNoConn")
	}
}

func TestCloseWithOpenStreams(t *testing.T) {
	ctx := context.Background()
	swarms := makeSwarms(ctx, t, 2)
	connectSwarms(t, ctx, swarms)

	s, err := swarms[0].NewStream(ctx, swarms[1].LocalPeer())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// close swarm before stream.
	err = swarms[0].Close()
	if err != nil {
		t.Fatal(err)
	}
}
