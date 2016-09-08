package swarm

import (
	"context"
	"sync"

	peer "github.com/ipfs/go-libp2p-peer"
)

type DialFunc func(context.Context, peer.ID) (*Conn, error)

func NewDialSync(dfn DialFunc) *DialSync {
	return &DialSync{
		dials:    make(map[peer.ID]*activeDial),
		dialFunc: dfn,
	}
}

type DialSync struct {
	dials    map[peer.ID]*activeDial
	dialsLk  sync.Mutex
	dialFunc DialFunc
}

type activeDial struct {
	id       peer.ID
	refCnt   int
	refCntLk sync.Mutex
	cancel   func()

	err    error
	conn   *Conn
	waitch chan struct{}

	ds *DialSync
}

func (dr *activeDial) wait(ctx context.Context) (*Conn, error) {
	defer dr.decref()
	select {
	case <-dr.waitch:
		return dr.conn, dr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ad *activeDial) incref() {
	ad.refCntLk.Lock()
	defer ad.refCntLk.Unlock()
	ad.refCnt++
}

func (ad *activeDial) decref() {
	ad.refCntLk.Lock()
	defer ad.refCntLk.Unlock()
	ad.refCnt--
	if ad.refCnt <= 0 {
		ad.cancel()
		ad.ds.dialsLk.Lock()
		delete(ad.ds.dials, ad.id)
		ad.ds.dialsLk.Unlock()
	}
}

func (ds *DialSync) DialLock(ctx context.Context, p peer.ID) (*Conn, error) {
	ds.dialsLk.Lock()

	actd, ok := ds.dials[p]
	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		actd = &activeDial{
			id:     p,
			cancel: cancel,
			waitch: make(chan struct{}),
			ds:     ds,
		}
		ds.dials[p] = actd

		go func(ctx context.Context, p peer.ID, ad *activeDial) {
			ad.conn, ad.err = ds.dialFunc(ctx, p)
			close(ad.waitch)
			ad.cancel()
			ad.waitch = nil // to ensure nobody tries reusing this
		}(ctx, p, actd)
	}

	actd.incref()
	ds.dialsLk.Unlock()

	return actd.wait(ctx)
}
