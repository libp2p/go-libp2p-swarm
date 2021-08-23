package swarm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
)

func getMockDialFunc() (dialWorkerFunc, func(), context.Context, <-chan struct{}) {
	dfcalls := make(chan struct{}, 512) // buffer it large enough that we won't care
	dialctx, cancel := context.WithCancel(context.Background())
	ch := make(chan struct{})
	f := func(p peer.ID, reqch <-chan dialRequest) {
		defer cancel()
		dfcalls <- struct{}{}
		go func() {
			for req := range reqch {
				<-ch
				req.resch <- dialResponse{conn: new(Conn)}
			}
		}()
	}

	var once sync.Once
	return f, func() { once.Do(func() { close(ch) }) }, dialctx, dfcalls
}

func TestBasicDialSync(t *testing.T) {
	df, done, _, callsch := getMockDialFunc()
	dsync := newDialSync(df)
	p := peer.ID("testpeer")

	finished := make(chan struct{}, 2)
	go func() {
		if _, err := dsync.Dial(context.Background(), p); err != nil {
			t.Error(err)
		}
		finished <- struct{}{}
	}()

	go func() {
		if _, err := dsync.Dial(context.Background(), p); err != nil {
			t.Error(err)
		}
		finished <- struct{}{}
	}()

	// short sleep just to make sure we've moved around in the scheduler
	time.Sleep(time.Millisecond * 20)
	done()

	<-finished
	<-finished

	if len(callsch) > 1 {
		t.Fatal("should only have called dial func once!")
	}
}

func TestDialSyncCancel(t *testing.T) {
	df, done, _, dcall := getMockDialFunc()

	dsync := newDialSync(df)

	p := peer.ID("testpeer")

	ctx1, cancel1 := context.WithCancel(context.Background())

	finished := make(chan struct{})
	go func() {
		_, err := dsync.Dial(ctx1, p)
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
		_, err := dsync.Dial(context.Background(), p)
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

	dsync := newDialSync(df)
	p := peer.ID("testpeer")
	ctx, cancel := context.WithCancel(context.Background())

	finished := make(chan struct{})
	go func() {
		if _, err := dsync.Dial(ctx, p); err != ctx.Err() {
			t.Error("should have gotten context error")
		}
		finished <- struct{}{}
	}()

	// Add a second dialwait in so two actors are waiting on the same dial
	go func() {
		if _, err := dsync.Dial(ctx, p); err != ctx.Err() {
			t.Error("should have gotten context error")
		}
		finished <- struct{}{}
	}()

	cancel()
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
	if _, err := dsync.Dial(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestFailFirst(t *testing.T) {
	var count int32
	f := func(p peer.ID, reqch <-chan dialRequest) {
		go func() {
			for {
				req, ok := <-reqch
				if !ok {
					return
				}

				if atomic.LoadInt32(&count) > 0 {
					req.resch <- dialResponse{conn: new(Conn)}
				} else {
					req.resch <- dialResponse{err: fmt.Errorf("gophers ate the modem")}
				}
				atomic.AddInt32(&count, 1)
			}
		}()
	}

	ds := newDialSync(f)
	p := peer.ID("testing")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	if _, err := ds.Dial(ctx, p); err == nil {
		t.Fatal("expected gophers to have eaten the modem")
	}

	c, err := ds.Dial(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("should have gotten a 'real' conn back")
	}
}

func TestStressActiveDial(t *testing.T) {
	ds := newDialSync(func(p peer.ID, reqch <-chan dialRequest) {
		go func() {
			for {
				req, ok := <-reqch
				if !ok {
					return
				}
				req.resch <- dialResponse{}
			}
		}()
	})

	wg := sync.WaitGroup{}

	pid := peer.ID("foo")

	makeDials := func() {
		for i := 0; i < 10000; i++ {
			ds.Dial(context.Background(), pid)
		}
		wg.Done()
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go makeDials()
	}

	wg.Wait()
}
