package swarm

import (
	"context"
	"time"

	ma "github.com/multiformats/go-multiaddr"
)

const P_CIRCUIT = 290

const numTiers = 2

var TierDelay = 1 * time.Second

// delayDialAddrs returns a address channel sorted by priority, pushing the
// addresses with delay between them. The other channel can be used to trigger
// sending more addresses in case all previous failed
func delayDialAddrs(ctx context.Context, c <-chan ma.Multiaddr) (<-chan ma.Multiaddr, chan<- struct{}) {
	out := make(chan ma.Multiaddr)
	triggerNext := make(chan struct{}, 1)

	go doDelayDialAddrs(ctx, c, out, triggerNext)

	return out, triggerNext
}

func doDelayDialAddrs(ctx context.Context, c <-chan ma.Multiaddr, out chan ma.Multiaddr, triggerNext chan struct{}) {
	delay := time.NewTimer(TierDelay)

	defer delay.Stop()
	defer close(out)
	var pending [numTiers][]ma.Multiaddr
	lastTier := -1

	// put enqueues the multiaddr
	put := func(addr ma.Multiaddr) {
		tier := getTier(addr)
		pending[tier] = append(pending[tier], addr)
	}

	// get gets the best (lowest tier) multiaddr available
	// note that within a single tier put/get behave like a stack (LIFO)
	get := func() (ma.Multiaddr, int) {
		for i, tier := range pending[:] {
			if len(tier) > 0 {
				addr := tier[len(tier)-1]
				tier[len(tier)-1] = nil
				pending[i] = tier[:len(tier)-1]
				return addr, i
			}
		}
		return nil, -1
	}

	// fillBuckets reads pending addresses form the channel without blocking
	fillBuckets := func() bool {
		for {
			select {
			case addr, ok := <-c:
				if !ok {
					return false
				}
				put(addr)
			default:
				return true
			}
		}
	}

	// waitForMore waits for addresses from the channel
	waitForMore := func() (bool, error) {
		select {
		case addr, ok := <-c:
			if !ok {
				return false, nil
			}
			put(addr)
		case <-ctx.Done():
			return false, ctx.Err()
		}
		return true, nil
	}

	// maybeJumpTier will check if the address tier is changing and optionally
	// wait some time.
	maybeJumpTier := func(tier int, next ma.Multiaddr) (cont bool, brk bool, err error) {
		if tier > lastTier && lastTier != -1 {
			// Wait the delay (preempt with new addresses or when the dialer
			// requests more addresses)
			select {
			case addr, ok := <-c:
				put(next)
				if !ok {
					return false, true, nil
				}
				put(addr)
				return true, false, nil
			case <-delay.C:
				delay.Reset(TierDelay)
			case <-triggerNext:
				if !delay.Stop() {
					<-delay.C
				}
				delay.Reset(TierDelay)
			case <-ctx.Done():
				return false, false, ctx.Err()
			}
		}

		// Note that we want to only update the tier after we've done the waiting
		// or we were asked to finish early
		lastTier = tier
		return false, false, nil
	}

	recvOrSend := func(next ma.Multiaddr) (brk bool, err error) {
		select {
		case addr, ok := <-c:
			put(next)
			if !ok {
				return true, nil
			}
			put(addr)
		case out <- next:
			// Always count the timeout since the last dial.
			if !delay.Stop() {
				<-delay.C
			}
			delay.Reset(TierDelay)
		case <-ctx.Done():
			return false, ctx.Err()
		}
		return false, nil
	}

	// process the address stream
	for {
		if !fillBuckets() {
			break // input channel closed
		}

		next, tier := get()

		// Nothing? Block!
		if next == nil {
			ok, err := waitForMore()
			if err != nil {
				return
			}
			if !ok {
				break // input channel closed
			}
			continue
		}

		cont, brk, err := maybeJumpTier(tier, next)
		if cont {
			continue // received an address while waiting, in case it's lower tier
			// look at it immediately
		}
		if brk {
			break // input channel closed
		}
		if err != nil {
			return
		}

		brk, err = recvOrSend(next)
		if brk {
			break // input channel closed
		}
		if err != nil {
			return
		}
	}

	// the channel is closed by now
	c = nil

	// finish sending
	for {
		next, tier := get()
		if next == nil {
			return
		}

		_, _, err := maybeJumpTier(tier, next)
		if err != nil {
			return
		}

		_, err = recvOrSend(next)
		if err != nil {
			return
		}
	}
}

// getTier returns the priority tier of the address.
// return value must be > 0 & < numTiers.
func getTier(addr ma.Multiaddr) int {
	switch {
	case isRelayAddr(addr):
		return 1
	default:
		return 0
	}
}

func isRelayAddr(addr ma.Multiaddr) bool {
	_, err := addr.ValueForProtocol(P_CIRCUIT)
	return err == nil
}
