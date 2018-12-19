package swarm

import (
	"testing"
	"time"

	"context"
	ma "github.com/multiformats/go-multiaddr"
	"sync"
)

const (
	T0_A = "/ip4/127.0.0.1/tcp/4001"
	T0_B = "/ip4/127.0.0.1/tcp/4002"
	T0_C = "/ip4/127.0.0.1/tcp/4003"
	T0_D = "/ip4/127.0.0.1/tcp/4004"

	T1_A = "/p2p-circuit/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu"
	T1_B = "/p2p-circuit/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM"
	T1_C = "/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu/p2p-circuit/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM"
)

func prepare() {
	var circuitProto = ma.Protocol{
		Code:  P_CIRCUIT,
		Size:  0,
		Name:  "p2p-circuit",
		VCode: ma.CodeToVarint(P_CIRCUIT),
	}
	_ = ma.AddProtocol(circuitProto)

	TierDelay = 32 * time.Millisecond // 2x windows timer resolution
}

// addrChan creates a multiaddr channel with `nsync` size. If nsync is larger
// than 0, the entries will get pre-buffered in the channel.
// addrDelays is a set of addresses and delays between sending them. If a string
// starts with '/' it will be parsed as an address and sent to the channel.
// Otherwise it will get parsed as a time to sleep before sending next addresses
// or closing the channel
func addrChan(t *testing.T, nsync int, addrDelays ...string) <-chan ma.Multiaddr {
	out := make(chan ma.Multiaddr, nsync)
	c := sync.NewCond(&sync.Mutex{})
	c.L.Lock()

	go func() {
		defer close(out)
		n := 0

		for _, ad := range addrDelays {
			if ad[0] != '/' {
				delay, err := time.ParseDuration(ad)
				if err != nil {
					t.Fatal(err)
				}

				time.Sleep(delay)
				continue
			}

			addr, err := ma.NewMultiaddr(ad)
			if err != nil {
				t.Fatal(err)
			}

			out <- addr

			n++
			if n == nsync {
				c.L.Lock()
				c.Broadcast()
				c.L.Unlock()
			}
		}
	}()

	if nsync != 0 {
		c.Wait()
	}
	c.L.Unlock()

	return out
}

func TestRelayMatch(t *testing.T) {
	prepare()

	addr, err := ma.NewMultiaddr(T0_A)
	if err != nil {
		t.Fatal(err)
	}

	if isRelayAddr(addr) {
		t.Error("T0_A shouldn't match")
	}

	addr, err = ma.NewMultiaddr(T1_A)
	if err != nil {
		t.Fatal(err)
	}

	if !isRelayAddr(addr) {
		t.Error("T1_A should match")
	}

	addr, err = ma.NewMultiaddr(T1_C)
	if err != nil {
		t.Fatal(err)
	}

	if !isRelayAddr(addr) {
		t.Error("T1_C should match")
	}
}

func TestDelayBasic(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 0, T0_A))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Errorf("it took longer than expected to get the address (%s)", time.Now().Sub(start).String())
	}
}

func TestDelaySingleT1(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 0, T1_A))

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}
}

func TestDelaySimpleT0T1(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 2, T0_A, T1_A))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 16*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}
}

func TestDelaySimpleT1T0(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 2, T1_A, T0_A))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 16*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}
}

func TestDelayedT1T0(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 0, T1_A, "16ms", T0_A))

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 16*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}

	if time.Now().Sub(start) > 32*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}
}

func TestDelayMoreT1T0(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, more := delayDialAddrs(ctx, addrChan(t, 2, T1_A, T0_A))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	more <- struct{}{}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}
}

func TestDelaySingleT0T1WaitT0(t *testing.T) {
	ctx := context.Background()
	prepare()
	TierDelay = 64 * time.Millisecond // 4x windows timer resolution

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 2, T0_A, T1_A, "16ms", T0_B))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T0_B) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 16*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}

	if time.Now().Sub(start) > 32*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 64*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}
}

func TestDelaySingleT0T1Wait(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 2, T0_A, T1_A, "16ms"))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 32*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}
}

func TestDelaySingleT0T1WaitLong(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 2, T0_A, T1_A, "64ms"))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 32*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}
}

func TestDelaySingleT0T1WaitTrigger(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, more := delayDialAddrs(ctx, addrChan(t, 2, T0_A, T1_A, "64ms"))

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 16*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	more <- struct{}{}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 32*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}
}

func TestDelaySingleT0WaitT1(t *testing.T) {
	ctx := context.Background()
	prepare()

	start := time.Now()
	ch, _ := delayDialAddrs(ctx, addrChan(t, 0, T0_A, "16ms", T1_A))

	time.Sleep(32 * time.Millisecond)

	if !recvPath(t, ch, T0_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) > 48*time.Millisecond {
		t.Error("it took longer than expected to get the address")
	}

	if !recvPath(t, ch, T1_A) {
		t.Error("expected to recieve a path")
	}

	if time.Now().Sub(start) < 32*time.Millisecond {
		t.Error("it took shorter than expected to get the address")
	}
}

func recvPath(t *testing.T, c <-chan ma.Multiaddr, expect string) bool {
	a, ok := <-c
	if !ok {
		return ok
	}

	if a == nil {
		t.Error("got nil path")
	}

	if a.String() != expect {
		t.Errorf("paths didn't match: %s - %s", expect, a.String())
	}

	return ok
}
