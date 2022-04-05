package swarm_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	. "github.com/libp2p/go-libp2p-swarm"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	testutil "github.com/libp2p/go-libp2p-core/test"
	swarmt "github.com/libp2p/go-libp2p-swarm/testing"
	"github.com/libp2p/go-libp2p-testing/ci"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/stretchr/testify/require"
)

func closeSwarms(swarms []*Swarm) {
	for _, s := range swarms {
		s.Close()
	}
}

func TestBasicDialPeer(t *testing.T) {
	t.Parallel()

	swarms := makeSwarms(t, 2)
	defer closeSwarms(swarms)
	s1 := swarms[0]
	s2 := swarms[1]

	s1.Peerstore().AddAddrs(s2.LocalPeer(), s2.ListenAddresses(), peerstore.PermanentAddrTTL)

	c, err := s1.DialPeer(context.Background(), s2.LocalPeer())
	require.NoError(t, err)

	s, err := c.NewStream(context.Background())
	require.NoError(t, err)
	s.Close()
}

func TestDialWithNoListeners(t *testing.T) {
	t.Parallel()

	s1 := makeDialOnlySwarm(t)
	swarms := makeSwarms(t, 1)
	defer closeSwarms(swarms)
	s2 := swarms[0]

	s1.Peerstore().AddAddrs(s2.LocalPeer(), s2.ListenAddresses(), peerstore.PermanentAddrTTL)

	c, err := s1.DialPeer(context.Background(), s2.LocalPeer())
	require.NoError(t, err)

	s, err := c.NewStream(context.Background())
	require.NoError(t, err)
	s.Close()
}

func acceptAndHang(l net.Listener) {
	conns := make([]net.Conn, 0, 10)
	for {
		c, err := l.Accept()
		if err != nil {
			break
		}
		if c != nil {
			conns = append(conns, c)
		}
	}
	for _, c := range conns {
		c.Close()
	}
}

func TestSimultDials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	swarms := makeSwarms(t, 2, swarmt.OptDisableReuseport)
	defer closeSwarms(swarms)

	// connect everyone
	{
		var wg sync.WaitGroup
		connect := func(s *Swarm, dst peer.ID, addr ma.Multiaddr) {
			// copy for other peer
			log.Debugf("TestSimultOpen: connecting: %s --> %s (%s)", s.LocalPeer(), dst, addr)
			s.Peerstore().AddAddr(dst, addr, peerstore.TempAddrTTL)
			if _, err := s.DialPeer(ctx, dst); err != nil {
				t.Error("error swarm dialing to peer", err)
			}
			wg.Done()
		}

		ifaceAddrs0, err := swarms[0].InterfaceListenAddresses()
		if err != nil {
			t.Fatal(err)
		}
		ifaceAddrs1, err := swarms[1].InterfaceListenAddresses()
		if err != nil {
			t.Fatal(err)
		}

		log.Info("Connecting swarms simultaneously.")
		for i := 0; i < 10; i++ { // connect 10x for each.
			wg.Add(2)
			go connect(swarms[0], swarms[1].LocalPeer(), ifaceAddrs1[0])
			go connect(swarms[1], swarms[0].LocalPeer(), ifaceAddrs0[0])
		}
		wg.Wait()
	}

	// should still just have 1, at most 2 connections :)
	c01l := len(swarms[0].ConnsToPeer(swarms[1].LocalPeer()))
	if c01l > 2 {
		t.Error("0->1 has", c01l)
	}
	c10l := len(swarms[1].ConnsToPeer(swarms[0].LocalPeer()))
	if c10l > 2 {
		t.Error("1->0 has", c10l)
	}
}

func newSilentPeer(t *testing.T) (peer.ID, ma.Multiaddr, net.Listener) {
	dst := testutil.RandPeerIDFatal(t)
	lst, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr, err := manet.FromNetAddr(lst.Addr())
	if err != nil {
		t.Fatal(err)
	}
	addrs, err := manet.ResolveUnspecifiedAddresses([]ma.Multiaddr{addr}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("new silent peer:", dst, addrs[0])
	return dst, addrs[0], lst
}

func TestDialWait(t *testing.T) {
	const dialTimeout = 250 * time.Millisecond

	swarms := makeSwarms(t, 1, swarmt.DialTimeout(dialTimeout))
	s1 := swarms[0]
	defer s1.Close()

	// dial to a non-existent peer.
	s2p, s2addr, s2l := newSilentPeer(t)
	go acceptAndHang(s2l)
	defer s2l.Close()
	s1.Peerstore().AddAddr(s2p, s2addr, peerstore.PermanentAddrTTL)

	before := time.Now()
	if c, err := s1.DialPeer(context.Background(), s2p); err == nil {
		defer c.Close()
		t.Fatal("error swarm dialing to unknown peer worked...", err)
	} else {
		t.Log("correctly got error:", err)
	}
	duration := time.Since(before)

	if duration < dialTimeout*DialAttempts {
		t.Error("< dialTimeout * DialAttempts not being respected", duration, dialTimeout*DialAttempts)
	}
	if duration > 2*dialTimeout*DialAttempts {
		t.Error("> 2*dialTimeout * DialAttempts not being respected", duration, 2*dialTimeout*DialAttempts)
	}

	if !s1.Backoff().Backoff(s2p, s2addr) {
		t.Error("s2 should now be on backoff")
	}
}

func TestDialBackoff(t *testing.T) {
	if ci.IsRunning() {
		t.Skip("travis will never have fun with this test")
	}
	t.Parallel()

	const dialTimeout = 100 * time.Millisecond

	ctx := context.Background()
	swarms := makeSwarms(t, 2, swarmt.DialTimeout(dialTimeout))
	defer closeSwarms(swarms)
	s1 := swarms[0]
	s2 := swarms[1]

	s2addrs, err := s2.InterfaceListenAddresses()
	require.NoError(t, err)
	s1.Peerstore().AddAddrs(s2.LocalPeer(), s2addrs, peerstore.PermanentAddrTTL)

	// dial to a non-existent peer.
	s3p, s3addr, s3l := newSilentPeer(t)
	go acceptAndHang(s3l)
	defer s3l.Close()
	s1.Peerstore().AddAddr(s3p, s3addr, peerstore.PermanentAddrTTL)

	// in this test we will:
	//   1) dial 10x to each node.
	//   2) all dials should hang
	//   3) s1->s2 should succeed.
	//   4) s1->s3 should not (and should place s3 on backoff)
	//   5) disconnect entirely
	//   6) dial 10x to each node again
	//   7) s3 dials should all return immediately (except 1)
	//   8) s2 dials should all hang, and succeed
	//   9) last s3 dial ends, unsuccessful

	dialOnlineNode := func(dst peer.ID, times int) <-chan bool {
		ch := make(chan bool)
		for i := 0; i < times; i++ {
			go func() {
				if _, err := s1.DialPeer(ctx, dst); err != nil {
					t.Error("error dialing", dst, err)
					ch <- false
				} else {
					ch <- true
				}
			}()
		}
		return ch
	}

	dialOfflineNode := func(dst peer.ID, times int) <-chan bool {
		ch := make(chan bool)
		for i := 0; i < times; i++ {
			go func() {
				if c, err := s1.DialPeer(ctx, dst); err != nil {
					ch <- false
				} else {
					t.Error("succeeded in dialing", dst)
					ch <- true
					c.Close()
				}
			}()
		}
		return ch
	}

	{
		// 1) dial 10x to each node.
		N := 10
		s2done := dialOnlineNode(s2.LocalPeer(), N)
		s3done := dialOfflineNode(s3p, N)

		// when all dials should be done by:
		dialTimeout1x := time.After(dialTimeout)
		dialTimeout10Ax := time.After(dialTimeout * 2 * 10) // DialAttempts * 10)

		// 2) all dials should hang
		select {
		case <-s2done:
			t.Error("s2 should not happen immediately")
		case <-s3done:
			t.Error("s3 should not happen yet")
		case <-time.After(time.Millisecond):
			// s2 may finish very quickly, so let's get out.
		}

		// 3) s1->s2 should succeed.
		for i := 0; i < N; i++ {
			select {
			case r := <-s2done:
				if !r {
					t.Error("s2 should not fail")
				}
			case <-s3done:
				t.Error("s3 should not happen yet")
			case <-dialTimeout1x:
				t.Error("s2 took too long")
			}
		}

		select {
		case <-s2done:
			t.Error("s2 should have no more")
		case <-s3done:
			t.Error("s3 should not happen yet")
		case <-dialTimeout1x: // let it pass
		}

		// 4) s1->s3 should not (and should place s3 on backoff)
		// N-1 should finish before dialTimeout1x * 2
		for i := 0; i < N; i++ {
			select {
			case <-s2done:
				t.Error("s2 should have no more")
			case r := <-s3done:
				if r {
					t.Error("s3 should not succeed")
				}
			case <-(dialTimeout1x):
				if i < (N - 1) {
					t.Fatal("s3 took too long")
				}
				t.Log("dialTimeout1x * 1.3 hit for last peer")
			case <-dialTimeout10Ax:
				t.Fatal("s3 took too long")
			}
		}

		// check backoff state
		if s1.Backoff().Backoff(s2.LocalPeer(), s2addrs[0]) {
			t.Error("s2 should not be on backoff")
		}
		if !s1.Backoff().Backoff(s3p, s3addr) {
			t.Error("s3 should be on backoff")
		}

		// 5) disconnect entirely

		for _, c := range s1.Conns() {
			c.Close()
		}
		for i := 0; i < 100 && len(s1.Conns()) > 0; i++ {
			<-time.After(time.Millisecond)
		}
		if len(s1.Conns()) > 0 {
			t.Fatal("s1 conns must exit")
		}
	}

	{
		// 6) dial 10x to each node again
		N := 10
		s2done := dialOnlineNode(s2.LocalPeer(), N)
		s3done := dialOfflineNode(s3p, N)

		// when all dials should be done by:
		dialTimeout1x := time.After(dialTimeout)
		dialTimeout10Ax := time.After(dialTimeout * 2 * 10) // DialAttempts * 10)

		// 7) s3 dials should all return immediately (except 1)
		for i := 0; i < N-1; i++ {
			select {
			case <-s2done:
				t.Error("s2 should not succeed yet")
			case r := <-s3done:
				if r {
					t.Error("s3 should not succeed")
				}
			case <-dialTimeout1x:
				t.Fatal("s3 took too long")
			}
		}

		// 8) s2 dials should all hang, and succeed
		for i := 0; i < N; i++ {
			select {
			case r := <-s2done:
				if !r {
					t.Error("s2 should succeed")
				}
			// case <-s3done:
			case <-(dialTimeout1x):
				t.Fatal("s3 took too long")
			}
		}

		// 9) the last s3 should return, failed.
		select {
		case <-s2done:
			t.Error("s2 should have no more")
		case r := <-s3done:
			if r {
				t.Error("s3 should not succeed")
			}
		case <-dialTimeout10Ax:
			t.Fatal("s3 took too long")
		}

		// check backoff state (the same)
		if s1.Backoff().Backoff(s2.LocalPeer(), s2addrs[0]) {
			t.Error("s2 should not be on backoff")
		}
		if !s1.Backoff().Backoff(s3p, s3addr) {
			t.Error("s3 should be on backoff")
		}
	}
}

func TestDialBackoffClears(t *testing.T) {
	t.Parallel()

	const dialTimeout = 250 * time.Millisecond
	swarms := makeSwarms(t, 2, swarmt.DialTimeout(dialTimeout))
	defer closeSwarms(swarms)
	s1 := swarms[0]
	s2 := swarms[1]

	// use another address first, that accept and hang on conns
	_, s2bad, s2l := newSilentPeer(t)
	go acceptAndHang(s2l)
	defer s2l.Close()

	// phase 1 -- dial to non-operational addresses
	s1.Peerstore().AddAddr(s2.LocalPeer(), s2bad, peerstore.PermanentAddrTTL)

	before := time.Now()
	_, err := s1.DialPeer(context.Background(), s2.LocalPeer())
	require.Error(t, err, "dialing to broken addr worked...")
	duration := time.Since(before)

	if duration < dialTimeout*DialAttempts {
		t.Error("< dialTimeout * DialAttempts not being respected", duration, dialTimeout*DialAttempts)
	}
	if duration > 2*dialTimeout*DialAttempts {
		t.Error("> 2*dialTimeout * DialAttempts not being respected", duration, 2*dialTimeout*DialAttempts)
	}
	require.True(t, s1.Backoff().Backoff(s2.LocalPeer(), s2bad), "s2 should now be on backoff")

	// phase 2 -- add the working address. dial should succeed.
	ifaceAddrs1, err := s2.InterfaceListenAddresses()
	require.NoError(t, err)
	s1.Peerstore().AddAddrs(s2.LocalPeer(), ifaceAddrs1, peerstore.PermanentAddrTTL)

	// backoffs are per address, not peer
	c, err := s1.DialPeer(context.Background(), s2.LocalPeer())
	require.NoError(t, err)
	defer c.Close()
	require.False(t, s1.Backoff().Backoff(s2.LocalPeer(), s2bad), "s2 should no longer be on backoff")
}

func TestDialPeerFailed(t *testing.T) {
	t.Parallel()

	swarms := makeSwarms(t, 2, swarmt.DialTimeout(100*time.Millisecond))
	defer closeSwarms(swarms)
	testedSwarm, targetSwarm := swarms[0], swarms[1]

	const expectedErrorsCount = 5
	for i := 0; i < expectedErrorsCount; i++ {
		_, silentPeerAddress, silentPeerListener := newSilentPeer(t)
		go acceptAndHang(silentPeerListener)
		defer silentPeerListener.Close()

		testedSwarm.Peerstore().AddAddr(targetSwarm.LocalPeer(), silentPeerAddress, peerstore.PermanentAddrTTL)
	}

	_, err := testedSwarm.DialPeer(context.Background(), targetSwarm.LocalPeer())
	require.Error(t, err)

	// dial_test.go:508: correctly get a combined error: failed to dial PEER: all dials failed
	//   * [/ip4/127.0.0.1/tcp/46485] failed to negotiate security protocol: context deadline exceeded
	//   * [/ip4/127.0.0.1/tcp/34881] failed to negotiate security protocol: context deadline exceeded
	// ...

	dialErr, ok := err.(*DialError)
	if !ok {
		t.Fatalf("expected *DialError, got %T", err)
	}

	if len(dialErr.DialErrors) != expectedErrorsCount {
		t.Errorf("expected %d errors, got %d", expectedErrorsCount, len(dialErr.DialErrors))
	}
}

func TestDialExistingConnection(t *testing.T) {
	swarms := makeSwarms(t, 2)
	defer closeSwarms(swarms)
	s1 := swarms[0]
	s2 := swarms[1]

	s1.Peerstore().AddAddrs(s2.LocalPeer(), s2.ListenAddresses(), peerstore.PermanentAddrTTL)

	c1, err := s1.DialPeer(context.Background(), s2.LocalPeer())
	require.NoError(t, err)

	c2, err := s1.DialPeer(context.Background(), s2.LocalPeer())
	require.NoError(t, err)

	require.Equal(t, c1, c2, "expecting the same connection from both dials")
}

func newSilentListener(t *testing.T) ([]ma.Multiaddr, net.Listener) {
	lst, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr, err := manet.FromNetAddr(lst.Addr())
	if err != nil {
		t.Fatal(err)
	}
	addrs, err := manet.ResolveUnspecifiedAddresses([]ma.Multiaddr{addr}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return addrs, lst

}

func TestDialSimultaneousJoin(t *testing.T) {
	const dialTimeout = 250 * time.Millisecond

	swarms := makeSwarms(t, 2, swarmt.DialTimeout(dialTimeout))
	defer closeSwarms(swarms)
	s1 := swarms[0]
	s2 := swarms[1]

	s2silentAddrs, s2silentListener := newSilentListener(t)
	go acceptAndHang(s2silentListener)

	connch := make(chan network.Conn, 512)
	errs := make(chan error, 2)

	// start a dial to s2 through the silent addr
	go func() {
		s1.Peerstore().AddAddrs(s2.LocalPeer(), s2silentAddrs, peerstore.PermanentAddrTTL)

		c, err := s1.DialPeer(context.Background(), s2.LocalPeer())
		if err != nil {
			errs <- err
			connch <- nil
			return
		}

		t.Logf("first dial succedded; conn: %+v", c)

		connch <- c
		errs <- nil
	}()

	// wait a bit for the dial to take hold
	time.Sleep(100 * time.Millisecond)

	// start a second dial to s2 that uses the real s2 addrs
	go func() {
		s2addrs, err := s2.InterfaceListenAddresses()
		if err != nil {
			errs <- err
			return
		}
		s1.Peerstore().AddAddrs(s2.LocalPeer(), s2addrs[:1], peerstore.PermanentAddrTTL)

		c, err := s1.DialPeer(context.Background(), s2.LocalPeer())
		if err != nil {
			errs <- err
			connch <- nil
			return
		}

		t.Logf("second dial succedded; conn: %+v", c)

		connch <- c
		errs <- nil
	}()

	// wait for the second dial to finish
	c2 := <-connch

	// start a third dial to s2, this should get the existing connection from the successful dial
	go func() {
		c, err := s1.DialPeer(context.Background(), s2.LocalPeer())
		if err != nil {
			errs <- err
			connch <- nil
			return
		}

		t.Logf("third dial succedded; conn: %+v", c)

		connch <- c
		errs <- nil
	}()

	c3 := <-connch

	// raise any errors from the previous goroutines
	for i := 0; i < 3; i++ {
		require.NoError(t, <-errs)
	}

	if c2 != c3 {
		t.Fatal("expected c2 and c3 to be the same")
	}

	// next, the first dial to s2, using the silent addr should timeout; at this point the dial
	// will error but the last chance check will see the existing connection and return it
	select {
	case c1 := <-connch:
		if c1 != c2 {
			t.Fatal("expected c1 and c2 to be the same")
		}
	case <-time.After(2 * dialTimeout):
		t.Fatal("no connection from first dial")
	}
}

func TestDialSelf(t *testing.T) {
	t.Parallel()

	swarms := makeSwarms(t, 2)
	defer closeSwarms(swarms)
	s1 := swarms[0]

	_, err := s1.DialPeer(context.Background(), s1.LocalPeer())
	require.ErrorIs(t, err, ErrDialToSelf, "expected error from self dial")
}
