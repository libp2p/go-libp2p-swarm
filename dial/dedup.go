package dial

import (
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
)

type dialDedup struct {
	lk       sync.Mutex
	inflight map[peer.ID]*Request
}

var _ Preparer = (*dialDedup)(nil)

// NewDedup returns a dial deduplicator, whose role is to ensure that at most one dial
// to any given peer is active at any given time.
func NewDedup() Preparer {
	return &dialDedup{
		inflight: make(map[peer.ID]*Request),
	}
}

func (d *dialDedup) requestCallback(r *Request) {
	d.lk.Lock()
	defer d.lk.Unlock()

	delete(d.inflight, r.PeerID())
}

func (d *dialDedup) Prepare(req *Request) error {
	id := req.PeerID()

	d.lk.Lock()
	active, ok := d.inflight[id]
	if !ok {
		// if no other dials to this peer are underway, add ourselves, along with a callback to delete us.
		req.AddCallback("dedup", d.requestCallback)
		d.inflight[id] = req
		d.lk.Unlock()
		return nil
	}

	req.Debugf("deduplicating dial; another one is in progress")

	// If there's an active dial in progress, let's wait until it completes.
	d.lk.Unlock()
	select {
	case <-active.Await():
		req.Debugf("deduplicated dial fulfilled from another dial")
		// the other dial completed, copy over the result.
		_, _ = req.CompleteFrom(active)
	case <-req.Context().Done():
		req.Debugf("deduplicated dial was cancelled")
		// our timeout elapsed or the dial was cancelled; abort.
		_, _ = req.Complete(nil, req.Context().Err())
	}

	return nil
}
