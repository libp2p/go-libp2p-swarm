package swarm

import (
	"context"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	mafmt "github.com/whyrusleeping/mafmt"
)

const p_circuit = 290

const numTiers = 2
const tierDelay = 2 * time.Second

var relay = mafmt.Or(mafmt.Base(p_circuit), mafmt.And(mafmt.IPFS, mafmt.Base(p_circuit)))

// delayDialAddrs returns a address channel sorted by priority, pushing the
// addresses with delay between them. The other channel can be used to trigger
// sending more addresses in case all previous failed
func delayDialAddrs(ctx context.Context, c <-chan ma.Multiaddr) (<-chan ma.Multiaddr, chan<- struct{}) {
	out := make(chan ma.Multiaddr)
	delay := time.NewTimer(tierDelay)
	triggerNext := make(chan struct{}, 1)

	go func() {
		defer delay.Stop()
		defer close(out)
		var pending [numTiers][]ma.Multiaddr
		lastTier := 0

		// put enqueues the mutliaddr
		put := func(addr ma.Multiaddr) {
			tier := getTier(addr)
			pending[tier] = append(pending[tier], addr)
		}

		// get gets the best (lowest tier) multiaddr available
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

	outer:
		for {
		fill:
			for {
				select {
				case addr, ok := <-c:
					if !ok {
						break outer
					}
					put(addr)
				default:
					break fill
				}
			}

			next, tier := get()

			// Nothing? Block!
			if next == nil {
				select {
				case addr, ok := <-c:
					if !ok {
						break outer
					}
					put(addr)
				case <-ctx.Done():
					return
				}
				continue
			}

			// Jumping a tier?
			if tier > lastTier {
				// Wait the delay (preempt with new addresses or when the dialer
				// requests more addresses)
				select {
				case addr, ok := <-c:
					if !ok {
						break outer
					}
					put(addr)
					continue
				case <-delay.C:
					delay.Reset(tierDelay)
				case <-triggerNext:
					if !delay.Stop() {
						<-delay.C
					}
					delay.Reset(tierDelay)
				case <-ctx.Done():
					return
				}
			}

			lastTier = tier

			select {
			case addr, ok := <-c:
				put(next)
				if !ok {
					break outer
				}
				put(addr)
				continue
			case out <- next:
				// Always count the timeout since the last dial.
				if !delay.Stop() {
					<-delay.C
				}
				delay.Reset(tierDelay)
			case <-ctx.Done():
				return
			}
		}

		// finish sending
		for {
			next, tier := get()
			if next == nil {
				return
			}
			if tier > lastTier {
				select {
				case <-delay.C:
				case <-triggerNext:
					if !delay.Stop() {
						<-delay.C
					}
					delay.Reset(tierDelay)
				case <-ctx.Done():
					return
				}
			}
			tier = lastTier
			select {
			case out <- next:
				delay.Stop()
				delay.Reset(tierDelay)
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, triggerNext
}

// getTier returns the priority tier of the address.
// return value must be > 0 & < numTiers.
func getTier(addr ma.Multiaddr) int {
	switch {
	case relay.Matches(addr):
		return 1
	default:
		return 0
	}
}
