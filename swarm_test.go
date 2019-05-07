package swarm_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	stesting "github.com/libp2p/go-libp2p-swarm/testing"
)

var log = logging.Logger("swarm_test")

func SubtestSwarm(t *testing.T, SwarmNum int, MsgNum int) {
	// t.Skip("skipping for another test")

	ctx := context.Background()
	swarms := stesting.MakeSwarms(ctx, t, SwarmNum, stesting.OptDisableReuseport)

	// connect everyone
	stesting.ConnectSwarms(t, ctx, swarms)

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
	swarms := stesting.MakeSwarms(ctx, t, 5)

	gotconn := make(chan struct{}, 10)
	swarms[0].SetConnHandler(func(conn network.Conn) {
		gotconn <- struct{}{}
	})

	stesting.ConnectSwarms(t, ctx, swarms)

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

func TestAddrBlocking(t *testing.T) {
	ctx := context.Background()
	swarms := stesting.MakeSwarms(ctx, t, 2)

	swarms[0].SetConnHandler(func(conn network.Conn) {
		t.Errorf("no connections should happen! -- %s", conn)
	})

	_, block, err := net.ParseCIDR("127.0.0.1/8")
	if err != nil {
		t.Fatal(err)
	}

	swarms[1].Filters.AddDialFilter(block)

	swarms[1].Peerstore().AddAddr(swarms[0].LocalPeer(), swarms[0].ListenAddresses()[0], peerstore.PermanentAddrTTL)
	_, err = swarms[1].DialPeer(ctx, swarms[0].LocalPeer())
	if err == nil {
		t.Fatal("dial should have failed")
	}

	swarms[0].Peerstore().AddAddr(swarms[1].LocalPeer(), swarms[1].ListenAddresses()[0], peerstore.PermanentAddrTTL)
	_, err = swarms[0].DialPeer(ctx, swarms[1].LocalPeer())
	if err == nil {
		t.Fatal("dial should have failed")
	}
}

func TestFilterBounds(t *testing.T) {
	ctx := context.Background()
	swarms := stesting.MakeSwarms(ctx, t, 2)

	conns := make(chan struct{}, 8)
	swarms[0].SetConnHandler(func(conn network.Conn) {
		conns <- struct{}{}
	})

	// Address that we wont be dialing from
	_, block, err := net.ParseCIDR("192.0.0.1/8")
	if err != nil {
		t.Fatal(err)
	}

	// set filter on both sides, shouldnt matter
	swarms[1].Filters.AddDialFilter(block)
	swarms[0].Filters.AddDialFilter(block)

	stesting.ConnectSwarms(t, ctx, swarms)

	select {
	case <-time.After(time.Second):
		t.Fatal("should have gotten connection")
	case <-conns:
		t.Log("got connect")
	}
}

func TestNoDial(t *testing.T) {
	ctx := context.Background()
	swarms := stesting.MakeSwarms(ctx, t, 2)

	_, err := swarms[0].NewStream(network.WithNoDial(ctx, "swarm test"), swarms[1].LocalPeer())
	if err != network.ErrNoConn {
		t.Fatal("should have failed with ErrNoConn")
	}
}
