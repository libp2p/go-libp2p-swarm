package dial

import (
	"context"
	"sync"

	addrutil "github.com/libp2p/go-addr-util"
	peer "github.com/libp2p/go-libp2p-peer"
)

// ConcurrentFdDials is the number of concurrent outbound dials over transports
// that consume file descriptors
const ConcurrentFdDials = 160

// DefaultPerPeerRateLimit is the number of concurrent outbound dials to make
// per peer
const DefaultPerPeerRateLimit = 8

type throttler struct {
	lk sync.Mutex

	fdConsuming int
	fdLimit     int
	waitingOnFd []*Job

	activePerPeer      map[peer.ID]int
	perPeerLimit       int
	waitingOnPeerLimit map[peer.ID][]*Job

	finishedCh chan *Job

	peerTokenFreed chan peer.ID
	fdTokenFreed   chan struct{}

	inCh         <-chan *Job
	dialCh       chan<- *Job
	localCloseCh chan struct{}
}

var _ Throttler = (*throttler)(nil)

func (t *throttler) Start(ctx context.Context, inCh <-chan *Job, dialCh chan<- *Job) {
	t.inCh = inCh
	t.dialCh = dialCh

ThrottleLoop:
	for {
		// Process any token releases to avoid those channels getting backed up.
		select {
		case id := <-t.peerTokenFreed:
			waitlist := t.waitingOnPeerLimit[id]
			if len(waitlist) > 0 {
				next := waitlist[0]
				if len(waitlist) == 1 {
					delete(t.waitingOnPeerLimit, id)
				} else {
					waitlist[0] = nil // clear out memory
					t.waitingOnPeerLimit[id] = waitlist[1:]
				}

				t.activePerPeer[id]++
				if !t.throttleFd(next) {
					t.executeDial(next)
				}
			}
			continue ThrottleLoop

		case <-t.fdTokenFreed:
			if len(t.waitingOnFd) == 0 {
				continue ThrottleLoop
			}
			next := t.waitingOnFd[0]
			t.waitingOnFd[0] = nil // clear out memory
			t.waitingOnFd = t.waitingOnFd[1:]
			if len(t.waitingOnFd) == 0 {
				t.waitingOnFd = nil // clear out memory
			}
			t.fdConsuming++
			t.executeDial(next)
			continue ThrottleLoop

		default:
		}

		select {
		case j := <-t.inCh:
			if !t.throttlePeerLimit(j) && !t.throttleFd(j) {
				t.executeDial(j)
			}

		case j := <-t.finishedCh:
			id := j.req.id
			if addrutil.IsFDCostlyTransport(j.addr) {
				t.fdConsuming--
				t.fdTokenFreed <- struct{}{}
			}

			t.activePerPeer[id]--
			if t.activePerPeer[id] == 0 {
				delete(t.activePerPeer, id)
			}
			t.peerTokenFreed <- id

		case <-ctx.Done():
			return

		case <-t.localCloseCh:
			return
		}
	}
}

func (t *throttler) Close() error {
	close(t.localCloseCh)
	return nil
}

func (t *throttler) throttleFd(j *Job) (throttle bool) {
	if !addrutil.IsFDCostlyTransport(j.addr) {
		return false
	}
	if t.fdConsuming >= t.fdLimit {
		t.waitingOnFd = append(t.waitingOnFd, j)
		return true
	}
	t.fdConsuming++
	return false
}

func (t *throttler) throttlePeerLimit(j *Job) (throttle bool) {
	id := j.req.id
	if t.activePerPeer[id] >= t.perPeerLimit {
		wlist := t.waitingOnPeerLimit[id]
		t.waitingOnPeerLimit[id] = append(wlist, j)
		return true
	}
	t.activePerPeer[id]++
	return false
}

func NewDefaultThrottler() *throttler {
	return NewThrottlerWithParams(ConcurrentFdDials, DefaultPerPeerRateLimit)
}

func NewThrottlerWithParams(fdLimit, perPeerLimit int) *throttler {
	return &throttler{
		fdLimit:      fdLimit,
		perPeerLimit: perPeerLimit,

		waitingOnPeerLimit: make(map[peer.ID][]*Job),
		activePerPeer:      make(map[peer.ID]int),
		localCloseCh:       make(chan struct{}),
		finishedCh:         make(chan *Job, 100),
		fdTokenFreed:       make(chan struct{}, 1),
		peerTokenFreed:     make(chan peer.ID, 1),
	}
}

func (t *throttler) executeDial(j *Job) {
	if j.Cancelled() {
		t.finishedCh <- j
		return
	}

	j.req.AddCallback(func() {
		// clear peer dials
		// todo: this may be called as many times as dials have been issued per peer
		t.lk.Lock()
		defer t.lk.Unlock()
		delete(t.waitingOnPeerLimit, j.req.id)
	})

	j.AddCallback(func() {
		t.finishedCh <- j
	})

	t.dialCh <- j
}
