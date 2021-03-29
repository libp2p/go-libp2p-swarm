package swarm

import (
	"context"
	"errors"
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	ma "github.com/multiformats/go-multiaddr"
)

var errDialFailed = errors.New("dial failed")

// DialFunc is the type of function expected by DialSync.
type DialFunc func(context.Context, peer.ID, DialFilterFunc) (*Conn, error)

// DialFilterFunc is a function that filters a set of multiaddrs to trigger a new dial
type DialFilterFunc func([]ma.Multiaddr) []ma.Multiaddr

// NewDialSync constructs a new DialSync
func NewDialSync(dfn DialFunc) *DialSync {
	return &DialSync{
		dials:    make(map[peer.ID]*activeDial),
		dialFunc: dfn,
	}
}

// DialSync is a dial synchronization helper that ensures that at most one dial
// to any given peer is active at any given time.
type DialSync struct {
	dials    map[peer.ID]*activeDial
	dialsLk  sync.Mutex
	dialFunc DialFunc
}

type activeDial struct {
	id       peer.ID
	refCnt   int
	refCntLk sync.Mutex
	ctx      context.Context
	cancel   func()

	addrs   map[ma.Multiaddr]struct{}
	addrsLk sync.Mutex

	err    error
	conn   *Conn
	waitch chan struct{}
	connch chan *Conn
	errch  chan error
	dialch chan struct{}
	donech chan struct{}

	ds *DialSync
}

func (ad *activeDial) dial(ctx context.Context) (*Conn, error) {
	defer ad.decref()

	dialCtx := ad.dialContext(ctx)
	go func() {
		c, err := ad.ds.dialFunc(dialCtx, ad.id, ad.filter)

		if err != nil {
			select {
			case ad.errch <- err:
			case <-ad.ctx.Done():
			}

			return
		}

		select {
		case ad.connch <- c:
		case <-ad.ctx.Done():
			log.Debugf("dial context done; closing connection %+v", c)
			_ = c.Close()
		}
	}()

	select {
	case <-ad.waitch:
		return ad.conn, ad.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ad *activeDial) dialContext(ctx context.Context) context.Context {
	dialCtx := ad.ctx

	forceDirect, reason := network.GetForceDirectDial(ctx)
	if forceDirect {
		dialCtx = network.WithForceDirectDial(dialCtx, reason)
	}

	simConnect, reason := network.GetSimultaneousConnect(ctx)
	if simConnect {
		dialCtx = network.WithSimultaneousConnect(dialCtx, reason)
	}

	return dialCtx
}

func (ad *activeDial) filter(addrs []ma.Multiaddr) (result []ma.Multiaddr) {
	ad.addrsLk.Lock()
	defer ad.addrsLk.Unlock()

	for _, a := range addrs {
		_, active := ad.addrs[a]
		if active {
			continue
		}

		result = append(result, a)
		ad.addrs[a] = struct{}{}
	}

	return result
}

func (ad *activeDial) incref() {
	ad.refCntLk.Lock()
	defer ad.refCntLk.Unlock()
	ad.refCnt++
}

func (ad *activeDial) decref() {
	ad.refCntLk.Lock()
	ad.refCnt--
	maybeZero := (ad.refCnt <= 0)
	ad.refCntLk.Unlock()

	// make sure to always take locks in correct order.
	if maybeZero {
		ad.ds.dialsLk.Lock()
		ad.refCntLk.Lock()
		// check again after lock swap drop to make sure nobody else called incref
		// in between locks
		if ad.refCnt <= 0 {
			ad.cancel()
			close(ad.donech)
			delete(ad.ds.dials, ad.id)
		}
		ad.refCntLk.Unlock()
		ad.ds.dialsLk.Unlock()
	}
}

func (ad *activeDial) start(ctx context.Context) {
	defer ad.cancel()
	defer close(ad.waitch)

	dialCnt := 0
	for {
		select {
		case <-ad.dialch:
			dialCnt++

		case ad.conn = <-ad.connch:
			return
		case err := <-ad.errch:
			if err != errNoNewAddresses {
				ad.err = multierror.Append(ad.err, err)
			}

			dialCnt--
			if dialCnt == 0 {
				if ad.err == nil {
					ad.err = errDialFailed
				}

				return
			}

		case <-ctx.Done():
			if ad.err == nil {
				ad.err = errDialFailed
			}
			return
		}
	}
}

func (ds *DialSync) getActiveDial(p peer.ID) *activeDial {
	ds.dialsLk.Lock()
	defer ds.dialsLk.Unlock()

	actd, ok := ds.dials[p]
	if !ok {
		adctx, cancel := context.WithCancel(context.Background())
		actd = &activeDial{
			id:     p,
			ctx:    adctx,
			cancel: cancel,
			addrs:  make(map[ma.Multiaddr]struct{}),
			waitch: make(chan struct{}),
			connch: make(chan *Conn),
			errch:  make(chan error),
			dialch: make(chan struct{}),
			donech: make(chan struct{}),
			ds:     ds,
		}
		ds.dials[p] = actd

		go actd.start(adctx)
	}

	// increase ref count before dropping dialsLk
	actd.incref()

	return actd
}

// DialLock initiates a dial to the given peer if there are none in progress
// then waits for the dial to that peer to complete.
func (ds *DialSync) DialLock(ctx context.Context, p peer.ID) (*Conn, error) {
	var ad *activeDial

startDial:
	for {
		ad = ds.getActiveDial(p)

		// signal the start of dial
		select {
		case ad.dialch <- struct{}{}:
			break startDial
		case <-ad.waitch:
			// we lost a race, we need to try again because the connection might not be what we want
			ad.decref()

			select {
			case <-ad.donech:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return ad.dial(ctx)
}

// CancelDial cancels all in-progress dials to the given peer.
func (ds *DialSync) CancelDial(p peer.ID) {
	ds.dialsLk.Lock()
	defer ds.dialsLk.Unlock()
	if ad, ok := ds.dials[p]; ok {
		ad.cancel()
	}
}
