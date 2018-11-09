package dial

import (
	"errors"
	"sync"
	"time"

	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-peer"
)

var log = logging.Logger("swarm")

// ErrDialBackoff is returned by the backoff code when a given peer has
// been dialed too frequently
var ErrDialBackoff = errors.New("dial backoff")

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
// * It's safe to use it's zero value.
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

func (db *Backoff) Prepare(req *Request) {
	req.AddCallback(func() {
		if req.err != nil {
			db.AddBackoff(req.id)
			return
		}
		db.ClearBackoff(req.id)
	})

	// if this peer has been backed off, complete the dial immediately
	if !db.Backoff(req.id) {
		return
	}
	log.Event(req.ctx, "swarmDialBackoff", req.id)
	req.Complete(nil, ErrDialBackoff)
}

var _ Preparer = (*Backoff)(nil)

type backoffPeer struct {
	tries int
	until time.Time
}

// Backoff returns whether the client should backoff from dialing
// peer p
func (db *Backoff) Backoff(p peer.ID) (backoff bool) {
	db.lock.Lock()
	defer db.lock.Unlock()

	bp, found := db.entries[p]
	if found && time.Now().Before(bp.until) {
		return true
	}
	return false
}

// AddBackoff lets other nodes know that we've entered backoff with
// peer p, so dialers should not wait unnecessarily. We still will
// attempt to dial with one goroutine, in case we get through.
//
// Backoff is not exponential, it's quadratic and computed according to the
// following formula:
//
//     BackoffBase + BakoffCoef * PriorBackoffs^2
//
// Where PriorBackoffs is the number of previous backoffs.
func (db *Backoff) AddBackoff(p peer.ID) {
	db.lock.Lock()
	defer db.lock.Unlock()

	bp, ok := db.entries[p]
	if !ok {
		db.entries[p] = &backoffPeer{
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

// Clear removes a backoff record. Clients should call this after a
// successful Dial.
func (db *Backoff) ClearBackoff(p peer.ID) {
	db.lock.Lock()
	defer db.lock.Unlock()

	delete(db.entries, p)
}
