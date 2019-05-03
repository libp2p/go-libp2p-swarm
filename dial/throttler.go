package dial

import (
	"os"
	"strconv"

	addrutil "github.com/libp2p/go-addr-util"
	"github.com/libp2p/go-libp2p-core/peer"
)

// ConcurrentFdDials is the number of concurrent outbound dials over transports that consume file descriptors
const ConcurrentFdDials = 160

// DefaultPerPeerRateLimit is the number of concurrent outbound dials to make per peer
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
	peerTokenFreed chan peer.ID
	fdTokenFreed   chan struct{}
	closeCh        chan struct{}
}

var _ Throttler = (*throttler)(nil)

func NewThrottlerWithParams(fdLimit, perPeerLimit int) Throttler {
	return &throttler{
		fdLimit:   fdLimit,
		peerLimit: perPeerLimit,

		peerWaiters:   make(map[peer.ID][]*Job),
		peerConsuming: make(map[peer.ID]int),
		closeCh:       make(chan struct{}),

		// we are capped by file descriptor limits, so it's safe to use as our chan lengths.
		finished:       make(chan *Job, fdLimit),
		fdTokenFreed:   make(chan struct{}, fdLimit),
		peerTokenFreed: make(chan peer.ID, fdLimit),
	}
}

func NewDefaultThrottler() Throttler {
	fd := ConcurrentFdDials
	if env := os.Getenv("LIBP2P_SWARM_FD_LIMIT"); env != "" {
		if n, err := strconv.ParseInt(env, 10, 32); err == nil {
			fd = int(n)
		}
	}
	return NewThrottlerWithParams(fd, DefaultPerPeerRateLimit)
}

func (t *throttler) Run(incoming <-chan *Job, released chan<- *Job) {
	for {
		// process token releases first.
		select {
		case id := <-t.peerTokenFreed:
			t.peerConsuming[id]--
			if t.peerConsuming[id] == 0 {
				delete(t.peerConsuming, id)
			}
			log.Debugf("[throttler] freeing peer token; peer %s; active for peer: %d; waiting on peer limit: %d",
				id, t.peerConsuming[id], len(t.peerWaiters[id]))
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

		case <-t.fdTokenFreed:
			t.fdConsuming--
			log.Debugf("[throttler] freeing FD token; waiting: %d; consuming: %d", len(t.fdWaiters), t.fdConsuming)
			for len(t.fdWaiters) > 0 {
				next := t.fdWaiters[0]
				t.fdWaiters[0] = nil
				t.fdWaiters = t.fdWaiters[1:]
				if len(t.fdWaiters) == 0 {
					t.fdWaiters = nil
				}
				if next.Cancelled() {
					t.peerTokenFreed <- next.Request().PeerID()
					continue
				}
				t.fdConsuming++
				t.releaseJob(next, released)
				break
			}

		default:
		}

		select {
		case job := <-t.finished:
			id := job.Request().PeerID()
			log.Debugf("[throttler] clearing state for completed job; peer: %s; addr: %s", id, job.Address())
			if addrutil.IsFDCostlyTransport(job.Address()) {
				t.fdTokenFreed <- struct{}{}
			}
			t.peerTokenFreed <- id

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
	log.Debugf("[throttler] releasing dial job to executor: %v", job.Address())
	released <- job
}

func (t *throttler) throttleFd(job *Job) (throttle bool) {
	if !addrutil.IsFDCostlyTransport(job.Address()) {
		return false
	}
	if t.fdConsuming >= t.fdLimit {
		log.Debugf("[throttler] blocked dial waiting on FD token; peer: %s; addr: %s; consuming: %d; "+
			"limit: %d; waiting: %d", job.Request().PeerID(), job.Address(), t.fdConsuming, t.fdLimit, len(t.fdWaiters))
		t.fdWaiters = append(t.fdWaiters, job)
		job.MarkBlocked()
		return true
	}
	log.Debugf("[throttler] taking FD token: peer: %s; addr: %s; prev consuming: %d",
		job.Request().PeerID(), job.Address(), t.fdConsuming)
	t.fdConsuming++
	return false
}

func (t *throttler) throttlePeerLimit(job *Job) (throttle bool) {
	id := job.Request().PeerID()
	if t.peerConsuming[id] >= t.peerLimit {
		log.Debugf("[throttler] blocked dial waiting on peer limit; peer: %s; addr: %s; active: %d; "+
			"peer limit: %d; waiting: %d", id, job.Address(), t.peerConsuming[id], t.peerLimit, len(t.peerWaiters[id]))
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
