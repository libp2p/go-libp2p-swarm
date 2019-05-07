package dial

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
)

var (
	DefaultBackoffBase = time.Second * 5
	DefaultBackoffCoef = time.Second
	DefaultBackoffMax  = time.Minute * 5
)

type BackoffConfig struct {
	// BackoffBase is the base amount of time to backoff.
	BackoffBase time.Duration
	// BackoffCoef is the backoff coefficient.
	BackoffCoef time.Duration
	// BackoffMax is the maximum backoff time.
	BackoffMax time.Duration
}

func DefaultBackoffConfig() *BackoffConfig {
	return &BackoffConfig{
		BackoffBase: DefaultBackoffBase,
		BackoffCoef: DefaultBackoffCoef,
		BackoffMax:  DefaultBackoffMax,
	}
}

type backoffPeer struct {
	tries int
	until time.Time
}

type Backoff struct {
	config *BackoffConfig

	lock    sync.RWMutex
	entries map[peer.ID]*backoffPeer
}

var _ Preparer = (*Backoff)(nil)

// NewBackoff creates a Preparer used to avoid over-dialing the same, dead peers.
//
// It acts like a circuit-breaker, tracking failed dial requests and denying subsequent
// attempts if they occur during the backoff period.
//
// The backoff period starts with config.BackoffBase. When the next dial attempt is made
// after the backoff expires, we use that dial as a probe. If it succeeds, we clear the
// backoff entry. If it fails, we boost the duration of the existing entry according to
// the following quadratic formula:
//
//     BackoffBase + (BakoffCoef * PriorBackoffCount^2)
//
// Where PriorBackoffCount is the number of previous backoffs.
func NewBackoff(config *BackoffConfig) Preparer {
	return &Backoff{
		config:  config,
		entries: make(map[peer.ID]*backoffPeer),
	}
}

func (b *Backoff) Prepare(req *Request) error {
	// if this peer has been backed off, complete the dial immediately
	if b.Backoff(req.PeerID()) {
		req.Debugf("peer is backed off")
		log.Event(req.ctx, "swarmDialBackoff", req.PeerID())
		return ErrDialBackoff
	}

	req.AddCallback("backoff", b.requestCallback)
	return nil
}

// Backoff returns whether the client should backoff from dialing peer p
func (b *Backoff) Backoff(p peer.ID) (backoff bool) {
	b.lock.Lock()
	defer b.lock.Unlock()

	bp, found := b.entries[p]
	if found && time.Now().Before(bp.until) {
		return true
	}
	return false
}

// AddBackoff adds a new backoff entry for this peer, or boosts the backoff
// period if an entry already exists.
func (b *Backoff) AddBackoff(p peer.ID) {
	b.lock.Lock()
	defer b.lock.Unlock()

	bp, ok := b.entries[p]
	if !ok {
		b.entries[p] = &backoffPeer{
			tries: 1,
			until: time.Now().Add(b.config.BackoffBase),
		}
		return
	}

	backoffTime := b.config.BackoffBase + b.config.BackoffCoef*time.Duration(bp.tries*bp.tries)
	if backoffTime > b.config.BackoffMax {
		backoffTime = b.config.BackoffMax
	}
	bp.until = time.Now().Add(backoffTime)
	bp.tries++
}

// Clear removes a backoff record.
func (b *Backoff) ClearBackoff(p peer.ID) {
	b.lock.Lock()
	defer b.lock.Unlock()

	delete(b.entries, p)
}

func (b *Backoff) requestCallback(req *Request) {
	if _, err := req.Result(); err != nil && err != context.Canceled {
		req.Debugf("backing off")
		b.AddBackoff(req.PeerID())
	} else if err == nil {
		req.Debugf("clearing backoffs")
		b.ClearBackoff(req.PeerID())
	}
}
