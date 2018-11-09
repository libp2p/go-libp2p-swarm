package dial

import (
	"sync"

	peer "github.com/libp2p/go-libp2p-peer"
)

// NewDialSync constructs a new DialSync
func NewDialSync() *Sync {
	return &Sync{
		dials: make(map[peer.ID]*Request),
	}
}

// DialSync is a dial synchronization helper that ensures that at most one dial
// to any given peer is active at any given time.
type Sync struct {
	dialsLk sync.Mutex
	dials   map[peer.ID]*Request
}

var _ Preparer = (*Sync)(nil)

func (ds *Sync) Prepare(req *Request) {
	id := req.id

	ds.dialsLk.Lock()
	active, ok := ds.dials[id]

	// is there an active dial in progress? In that case, let's wait till it completes.
	if ok {
		ds.dialsLk.Unlock()
		select {
		case <-active.notifyCh:
			req.Complete(active.Values())
		case <-req.ctx.Done():
			req.Complete(nil, req.ctx.Err())
		}
		return
	}

	req.AddCallback(func() {
		ds.dialsLk.Lock()
		delete(ds.dials, id)
		ds.dialsLk.Unlock()
	})

	ds.dials[id] = req
	ds.dialsLk.Unlock()
}
