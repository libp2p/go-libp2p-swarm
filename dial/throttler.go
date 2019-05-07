package dial

import (
	"os"
	"strconv"

	addrutil "github.com/libp2p/go-addr-util"
	"github.com/libp2p/go-libp2p-core/peer"
)

// DefaultConcurrentFdDials is the number of concurrent outbound dials over transports that consume file descriptors.
const DefaultConcurrentFdDials = 160

// DefaultPerPeerRateLimit is the number of concurrent outbound dials to make per peer.
const DefaultPerPeerRateLimit = 8

type throttler struct {
	/* file descriptor throttling */
	fdConsuming int
	fdLimit     int
	fdWaiters   []*Job

	/* dials-per-peer throttling */
	peerConsuming map[peer.ID]int
	peerLimit     int
	peerWaiters   map[peer.ID][]*Job

	/* signalling channels */
	finished       chan *Job
	peerTokenFreed chan *Job
	fdTokenFreed   chan *Job
	closeCh        chan struct{}
}

var _ Throttler = (*throttler)(nil)

// NewThrottlerWithParams returns a throttler that limits concurrency based on two factors:
// 	(1) number of inflight dials per peer, and
// 	(2) total inflight dials with transports that consume file descriptors.
//
// The limits are enforced on a peer level first, then on the file descriptor level, using the
// the arguments as the maximum limits.
func NewThrottlerWithParams(fdLimit, perPeerLimit int) Throttler {
	return &throttler{
		fdLimit:   fdLimit,
		peerLimit: perPeerLimit,

		peerWaiters:   make(map[peer.ID][]*Job),
		peerConsuming: make(map[peer.ID]int),
		closeCh:       make(chan struct{}),

		// we are capped by file descriptor limits, so it's safe to use as our chan lengths.
		finished:       make(chan *Job, fdLimit),
		fdTokenFreed:   make(chan *Job, fdLimit),
		peerTokenFreed: make(chan *Job, fdLimit),
	}
}

// NewThrottler returns a Throttler just like NewThrottlerWithParams, using default limits, governed by constants:
//   * active peers per dial: 8
//   * file descriptors: 160
//
// The LIBP2P_SWARM_FD_LIMIT can be used to override the file descriptors limit.
func NewThrottler() Throttler {
	fd := DefaultConcurrentFdDials
	if env := os.Getenv("LIBP2P_SWARM_FD_LIMIT"); env != "" {
		if n, err := strconv.ParseInt(env, 10, 32); err == nil {
			fd = int(n)
		}
	}
	return NewThrottlerWithParams(fd, DefaultPerPeerRateLimit)
}

func (t *throttler) Run(incoming <-chan *Job, released chan<- *Job) {
	for {
		// First, process token releases.
		select {
		case job := <-t.peerTokenFreed:
			id := job.Request().PeerID()
			t.peerConsuming[id]--
			if t.peerConsuming[id] == 0 {
				delete(t.peerConsuming, id)
			}
			job.Debugf("throttler: freeing peer token; waiting on peer limit: %d", len(t.peerWaiters[id]))
			waitlist := t.peerWaiters[id]
			if len(waitlist) == 0 {
				continue
			}
			next := waitlist[0]
			if len(waitlist) == 1 {
				delete(t.peerWaiters, id)
			} else {
				waitlist[0] = nil
				t.peerWaiters[id] = waitlist[1:]
			}
			if next.Cancelled() {
				continue
			}
			t.peerConsuming[id]++
			if !t.throttleFd(next) {
				t.releaseJob(next, released)
			}
			continue

		case job := <-t.fdTokenFreed:
			t.fdConsuming--
			job.Debugf("throttler: freeing FD token; waiting: %d; consuming: %d", len(t.fdWaiters), t.fdConsuming)
			for len(t.fdWaiters) > 0 {
				next := t.fdWaiters[0]
				t.fdWaiters[0] = nil
				t.fdWaiters = t.fdWaiters[1:]
				if len(t.fdWaiters) == 0 {
					t.fdWaiters = nil
				}
				if next.Cancelled() {
					t.peerTokenFreed <- next
					continue
				}
				t.fdConsuming++
				t.releaseJob(next, released)
				break
			}

		default:
		}

		// Next, make forward progress.
		select {
		case job := <-t.finished:
			job.Debugf("throttler: clearing state for completed job")
			if addrutil.IsFDCostlyTransport(job.Address()) {
				t.fdTokenFreed <- job
			}
			t.peerTokenFreed <- job

		case job := <-incoming:
			if !t.throttlePeerLimit(job) && !t.throttleFd(job) {
				t.releaseJob(job, released)
			}

		case <-t.closeCh:
			return
		}
	}
}

func (t *throttler) releaseJob(job *Job, released chan<- *Job) {
	job.MarkInflight()
	job.AddCallback("throttler", func(j *Job) {
		t.finished <- j
	})
	job.Debugf("throttler: releasing dial job to executor")
	released <- job
}

func (t *throttler) throttleFd(job *Job) (throttle bool) {
	if !addrutil.IsFDCostlyTransport(job.Address()) {
		return false
	}
	if t.fdConsuming >= t.fdLimit {
		job.Debugf("throttler: blocked dial waiting on FD token; consuming: %d; limit: %d; waiting: %d",
			t.fdConsuming, t.fdLimit, len(t.fdWaiters))
		t.fdWaiters = append(t.fdWaiters, job)
		job.MarkBlocked()
		return true
	}
	job.Debugf("[throttler] taking FD token; prev consuming: %d", t.fdConsuming)
	t.fdConsuming++
	return false
}

func (t *throttler) throttlePeerLimit(job *Job) (throttle bool) {
	id := job.Request().PeerID()
	if t.peerConsuming[id] >= t.peerLimit {
		job.Debugf("throttler: blocked dial waiting on peer limit; active: %d; peer limit: %d; waiting: %d",
			t.peerConsuming[id], t.peerLimit, len(t.peerWaiters[id]))
		wlist := t.peerWaiters[id]
		t.peerWaiters[id] = append(wlist, job)
		job.MarkBlocked()
		return true
	}
	t.peerConsuming[id]++
	return false
}

func (t *throttler) Close() error {
	close(t.closeCh)
	return nil
}
