package dial

import (
	"sync"

	peer "github.com/libp2p/go-libp2p-peer"
)

type dialDedup struct {
	lk       sync.Mutex
	inflight map[peer.ID]*Request
}

var _ Preparer = (*dialDedup)(nil)

// NewDialDedup returns a dial deduplicator, whose role is to ensure that at most one dial
// to any given peer is active at any given time.
func NewDialDedup() Preparer {
	return &dialDedup{
		inflight: make(map[peer.ID]*Request),
	}
}

func (d *dialDedup) requestCallback(r *Request) {
	d.lk.Lock()
	delete(d.inflight, r.PeerID())
	d.lk.Unlock()
}

func (d *dialDedup) Prepare(req *Request) error {
	id := req.id

	d.lk.Lock()
	active, ok := d.inflight[id]
	if !ok {
		// if no other dials to this peer are underway, add ourselves, along with a callback to delete us.
		req.AddCallback("dedup_callback", d.requestCallback)
		d.inflight[id] = req
		d.lk.Unlock()
		return nil
	}

	// If there's an active dial in progress, let's wait until it completes.
	d.lk.Unlock()
	select {
	case <-active.Await():
		// the other dial completed, copy over the result.
		_, _ = req.CompleteFrom(active)
	case <-req.Context().Done():
		// our timeout elapsed or the dial was cancelled; abort.
		_, _ = req.Complete(nil, req.Context().Err())
	}

	return nil
}
