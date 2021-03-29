package swarm_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/libp2p/go-libp2p-swarm"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"

	ma "github.com/multiformats/go-multiaddr"
)

func getMockDialFunc() (DialFunc, func(), context.Context, <-chan struct{}) {
	dfcalls := make(chan struct{}, 512) // buffer it large enough that we won't care
	dialctx, cancel := context.WithCancel(context.Background())
	ch := make(chan struct{})
	f := func(ctx context.Context, p peer.ID, _ DialDedupFunc) (*Conn, error) {
		dfcalls <- struct{}{}
		defer cancel()
		select {
		case <-ch:
			return new(Conn), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	o := new(sync.Once)

	return f, func() { o.Do(func() { close(ch) }) }, dialctx, dfcalls
}

func getMockDialFuncWithPeerstore(ps peerstore.Peerstore) (DialFunc, func(), context.Context, <-chan *Conn) {
	dfcalls := make(chan *Conn, 512) // buffer it large enough that we won't care
	dialctx, cancel := context.WithCancel(context.Background())
	ch := make(chan struct{})
	f := func(ctx context.Context, p peer.ID, dedup DialDedupFunc) (*Conn, error) {
		defer cancel()

		addrs := dedup(ps.Addrs(p))
		if len(addrs) == 0 {
			return nil, ErrNoNewAddresses
		}

		select {
		case <-ch:
			c := new(Conn)
			dfcalls <- c
			return c, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	o := new(sync.Once)

	return f, func() { o.Do(func() { close(ch) }) }, dialctx, dfcalls
}

func TestBasicDialSync(t *testing.T) {
	ps := pstoremem.NewPeerstore()
	df, done, _, dfcalls := getMockDialFuncWithPeerstore(ps)

	dsync := NewDialSync(df)

	p := peer.ID("testpeer")
	ps.AddAddr(p, ma.StringCast("/ip4/1.2.3.4/tcp/5678"), peerstore.PermanentAddrTTL)

	ctx := context.Background()

	finished := make(chan *Conn)
	doDial := func() {
		c, err := dsync.DialLock(ctx, p)
		if err != nil {
			t.Error(err)
		}
		finished <- c
	}

	go doDial()
	go doDial()

	// short sleep just to make sure we've moved around in the scheduler
	time.Sleep(time.Millisecond * 20)
	done()

	c1 := <-finished
	c2 := <-finished

	if c1 != c2 {
		t.Fatal("should have gotten the same connection")
	}

	if len(dfcalls) != 1 {
		t.Fatal("expected a single dial call")
	}
}

func TestBasicDialSync2(t *testing.T) {
	ps := pstoremem.NewPeerstore()
	df, done, _, dfcalls := getMockDialFuncWithPeerstore(ps)

	dsync := NewDialSync(df)

	p := peer.ID("testpeer")

	ctx := context.Background()

	finished := make(chan *Conn)
	doDial := func(ctx context.Context, a ma.Multiaddr) {
		ps.AddAddr(p, a, peerstore.PermanentAddrTTL)
		c, err := dsync.DialLock(ctx, p)
		if err != nil {
			t.Error(err)
		}
		finished <- c
	}

	go doDial(ctx, ma.StringCast("/ip4/1.2.3.4/tcp/5678/p2p-circuit"))
	time.Sleep(10 * time.Millisecond)
	go doDial(network.WithForceDirectDial(ctx, "test"), ma.StringCast("/ip4/4.3.2.1/tcp/5678"))
	time.Sleep(10 * time.Millisecond)

	done()
	c1 := <-finished
	c2 := <-finished

	if c1 == c2 {
		t.Fatal("should have gotten different connections")
	}

	if len(dfcalls) != 2 {
		t.Fatal("expected two dial calls")
	}
}

func TestDialSyncCancel(t *testing.T) {
	df, done, _, dcall := getMockDialFunc()

	dsync := NewDialSync(df)

	p := peer.ID("testpeer")

	ctx1, cancel1 := context.WithCancel(context.Background())

	finished := make(chan struct{})
	go func() {
		_, err := dsync.DialLock(ctx1, p)
		if err != ctx1.Err() {
			t.Error("should have gotten context error")
		}
		finished <- struct{}{}
	}()

	// make sure the above makes it through the wait code first
	select {
	case <-dcall:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dial to start")
	}

	// Add a second dialwait in so two actors are waiting on the same dial
	go func() {
		_, err := dsync.DialLock(context.Background(), p)
		if err != nil {
			t.Error(err)
		}
		finished <- struct{}{}
	}()

	time.Sleep(time.Millisecond * 20)

	// cancel the first dialwait, it should not affect the second at all
	cancel1()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for wait to exit")
	}

	// short sleep just to make sure we've moved around in the scheduler
	time.Sleep(time.Millisecond * 20)
	done()

	<-finished
}

func TestDialSyncAllCancel(t *testing.T) {
	df, done, dctx, _ := getMockDialFunc()

	dsync := NewDialSync(df)

	p := peer.ID("testpeer")

	ctx1, cancel1 := context.WithCancel(context.Background())

	finished := make(chan struct{})
	go func() {
		_, err := dsync.DialLock(ctx1, p)
		if err != ctx1.Err() {
			t.Error("should have gotten context error")
		}
		finished <- struct{}{}
	}()

	// Add a second dialwait in so two actors are waiting on the same dial
	go func() {
		_, err := dsync.DialLock(ctx1, p)
		if err != ctx1.Err() {
			t.Error("should have gotten context error")
		}
		finished <- struct{}{}
	}()

	cancel1()
	for i := 0; i < 2; i++ {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for wait to exit")
		}
	}

	// the dial should have exited now
	select {
	case <-dctx.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dial to return")
	}

	// should be able to successfully dial that peer again
	done()
	_, err := dsync.DialLock(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFailFirst(t *testing.T) {
	var count int
	f := func(ctx context.Context, p peer.ID, _ DialDedupFunc) (*Conn, error) {
		if count > 0 {
			return new(Conn), nil
		}
		count++
		return nil, fmt.Errorf("gophers ate the modem")
	}

	ds := NewDialSync(f)

	p := peer.ID("testing")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	_, err := ds.DialLock(ctx, p)
	if err == nil {
		t.Fatal("expected gophers to have eaten the modem")
	}

	c, err := ds.DialLock(ctx, p)
	if err != nil {
		t.Fatal(err)
	}

	if c == nil {
		t.Fatal("should have gotten a 'real' conn back")
	}
}

func TestStressActiveDial(t *testing.T) {
	ds := NewDialSync(func(ctx context.Context, p peer.ID, _ DialDedupFunc) (*Conn, error) {
		return nil, nil
	})

	wg := sync.WaitGroup{}

	pid := peer.ID("foo")

	makeDials := func() {
		for i := 0; i < 10000; i++ {
			ds.DialLock(context.Background(), pid)
		}
		wg.Done()
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go makeDials()
	}

	wg.Wait()
}
