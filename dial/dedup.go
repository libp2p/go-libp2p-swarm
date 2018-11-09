package dial

import (
	"sync"

	peer "github.com/libp2p/go-libp2p-peer"
)

// NewDialDedup returns a dial deduplicator, whose role is to ensure that at most one dial
// to any given peer is active at any given time.
func NewDialDedup() Preparer {
	return &dialDedup{
		inflight: make(map[peer.ID]*Request),
	}
}

type dialDedup struct {
	lk       sync.Mutex
	inflight map[peer.ID]*Request
}

var _ Preparer = (*dialDedup)(nil)

func (ds *dialDedup) Prepare(req *Request) {
	id := req.id

	ds.lk.Lock()
	active, ok := ds.inflight[id]

	if ok {
		// If there's an active dial in progress, let's wait until it completes.
		ds.lk.Unlock()
		select {
		case <-active.notifyCh:
			// the other dial completed, copy over the result.
			req.CompleteFrom(active)
		case <-req.ctx.Done():
			// our timeout elapsed or the dial was cancelled; abort.
			req.Complete(nil, req.ctx.Err())
		}
		return
	}

	// If no other dials to this peer are underway, add ourselves, along with a callback to delete us
	req.AddCallback(func() {
		ds.lk.Lock()
		defer ds.lk.Unlock()

		delete(ds.inflight, id)
	})

	ds.inflight[id] = req
	ds.lk.Unlock()
}
