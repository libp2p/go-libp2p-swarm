package dial

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
)

// BackoffBase is the base amount of time to backoff (default: 5s).
var BackoffBase = time.Second * 5

// BackoffCoef is the backoff coefficient (default: 1s).
var BackoffCoef = time.Second

// BackoffMax is the maximum backoff time (default: 5m).
var BackoffMax = time.Minute * 5

// Backoff is a struct used to avoid over-dialing the same, dead peers.
// Whenever we totally time out on a peer (all three attempts), we add them
// to dialbackoff. Then, whenevers goroutines would _wait_ (dialsync), they
// check dialbackoff. If it's there, they don't wait and exit promptly with
// an error. (the single goroutine that is actually dialing continues to
// dial). If a dial is successful, the peer is removed from backoff.
// Example:
//
//  for {
//  	if ok, wait := dialsync.Lock(p); !ok {
//  		if backoff.Backoff(p) {
//  			return errDialFailed
//  		}
//  		<-wait
//  		continue
//  	}
//  	defer dialsync.Unlock(p)
//  	c, err := actuallyDial(p)
//  	if err != nil {
//  		dialbackoff.AddBackoff(p)
//  		continue
//  	}
//  	dialbackoff.Clear(p)
//  }
//

// DialBackoff is a type for tracking peer dial backoffs.
//
// * It's safe to use its zero value.
// * It's thread-safe.
// * It's *not* safe to move this type after using.
type Backoff struct {
	entries map[peer.ID]*backoffPeer
	lock    sync.RWMutex
}

func NewBackoff() Preparer {
	return &Backoff{
		entries: make(map[peer.ID]*backoffPeer),
	}
}

func (b *Backoff) Prepare(req *Request) error {
	// if this peer has been backed off, complete the dial immediately
	if b.Backoff(req.PeerID()) {
		log.Event(req.ctx, "swarmDialBackoff", req.PeerID())
		return ErrDialBackoff
	}

	req.AddCallback("backoff", b.requestCallback)
	return nil
}

func (b *Backoff) requestCallback(req *Request) {
	if _, err := req.Result(); err != nil && err != context.Canceled {
		b.AddBackoff(req.PeerID())
	} else if err == nil {
		b.ClearBackoff(req.PeerID())
	}
}

type backoffPeer struct {
	tries int
	until time.Time
}

var _ Preparer = (*Backoff)(nil)

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
//
// Backoff is not exponential, it's quadratic and computed according to the
// following formula:
//
//     BackoffBase + BakoffCoef * PriorBackoffs^2
//
// Where PriorBackoffs is the number of previous backoffs.
func (b *Backoff) AddBackoff(p peer.ID) {
	b.lock.Lock()
	defer b.lock.Unlock()

	bp, ok := b.entries[p]
	if !ok {
		b.entries[p] = &backoffPeer{
			tries: 1,
			until: time.Now().Add(BackoffBase),
		}
		return
	}

	backoffTime := BackoffBase + BackoffCoef*time.Duration(bp.tries*bp.tries)
	if backoffTime > BackoffMax {
		backoffTime = BackoffMax
	}
	bp.until = time.Now().Add(backoffTime)
	bp.tries++
}

// Clear removes a backoff record. Clients should call this after a successful dial.
func (b *Backoff) ClearBackoff(p peer.ID) {
	b.lock.Lock()
	defer b.lock.Unlock()

	delete(b.entries, p)
}