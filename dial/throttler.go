package dial

import (
	addrutil "github.com/libp2p/go-addr-util"
	peer "github.com/libp2p/go-libp2p-peer"
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

		peerWaiters:    make(map[peer.ID][]*Job),
		peerConsuming:  make(map[peer.ID]int),
		closeCh:        make(chan struct{}),
		finished:       make(chan *Job, 128),
		fdTokenFreed:   make(chan struct{}, 1),
		peerTokenFreed: make(chan peer.ID, 1),
	}
}

func NewDefaultThrottler() Throttler {
	return NewThrottlerWithParams(ConcurrentFdDials, DefaultPerPeerRateLimit)
}

func (t *throttler) Run(incoming <-chan *Job, released chan<- *Job) {
	jobFinishedCb := func(job *Job) {
		t.finished <- job
	}

	for {
		// Process any token releases to avoid those channels getting backed up.
		select {
		case id := <-t.peerTokenFreed:
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

			t.peerConsuming[id]++
			if !t.throttleFd(next) {

				released <- next
			}
			continue

		case <-t.fdTokenFreed:
			if len(t.fdWaiters) == 0 {
				continue
			}
			next := t.fdWaiters[0]
			t.fdWaiters[0] = nil
			t.fdWaiters = t.fdWaiters[1:]
			if len(t.fdWaiters) == 0 {
				t.fdWaiters = nil
			}
			t.fdConsuming++

			next.AddCallback("throttler", jobFinishedCb)
			released <- next
			continue

		default:
		}

		select {
		case job := <-incoming:
			if !t.throttlePeerLimit(job) && !t.throttleFd(job) {
				job.AddCallback("throttler", jobFinishedCb)
				released <- job
			}

		case job := <-t.finished:
			id := job.Request().PeerID()
			if addrutil.IsFDCostlyTransport(job.addr) {
				t.fdConsuming--
				t.fdTokenFreed <- struct{}{}
			}

			t.peerConsuming[id]--
			if t.peerConsuming[id] == 0 {
				delete(t.peerConsuming, id)
			}
			t.peerTokenFreed <- id

		case <-t.closeCh:
			return
		}
	}
}

func (t *throttler) throttleFd(j *Job) (throttle bool) {
	if !addrutil.IsFDCostlyTransport(j.addr) {
		return false
	}
	if t.fdConsuming >= t.fdLimit {
		t.fdWaiters = append(t.fdWaiters, j)
		return true
	}
	t.fdConsuming++
	return false
}

func (t *throttler) throttlePeerLimit(j *Job) (throttle bool) {
	id := j.Request().PeerID()
	if t.peerConsuming[id] >= t.peerLimit {
		wlist := t.peerWaiters[id]
		t.peerWaiters[id] = append(wlist, j)
		return true
	}
	t.peerConsuming[id]++
	return false
}

func (t *throttler) Close() error {
	close(t.closeCh)
	return nil
}
