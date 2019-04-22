package dial

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/go-multierror"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	tpt "github.com/libp2p/go-libp2p-transport"
	"golang.org/x/xerrors"
)

type AddConnFn func(tc tpt.Conn) (inet.Conn, error)

// Pipeline is the heart of the dialer. It represents a series of atomical components wired together, each with a
// clearly-delimited responsibility. The pipeline orchestrates these components to implement the dialing functionality.
//
// The Pipeline has five components. We provide brief descriptions below. For additional info, refer to the godoc
// of the appropriate interfaces:
//
// - Preparer: runs preparatory actions prior to the dial execution. Actions include: deduplicating, populating
//	 		   timeouts, validating the peer ID, etc.
// - AddressResolver: populates the set of addresses for a peer.
// - Planner
// - Throttler
// - Executor
type Pipeline struct {
	lk  sync.RWMutex
	ctx context.Context

	network inet.Network

	preparer  Preparer
	resolver  AddressResolver
	planner   Planner
	throttler Throttler
	executor  Executor

	throttleCh chan *Job
	executeCh  chan *Job

	addConnFn AddConnFn
}

func NewPipeline(ctx context.Context, net inet.Network, addConnFn AddConnFn) *Pipeline {
	pipeline := &Pipeline{
		ctx:        ctx,
		network:    net,
		addConnFn:  addConnFn,
		throttleCh: make(chan *Job, 128),
		executeCh:  make(chan *Job, 128),
	}
	return pipeline
}

func (p *Pipeline) Start() {
	go p.executor.Run(p.executeCh)
	go p.throttler.Run(p.throttleCh, p.executeCh)
}

func (p *Pipeline) Close() error {
	var err *multierror.Error
	err = multierror.Append(p.throttler.Close())
	err = multierror.Append(p.executor.Close())
	return err
}

func (p *Pipeline) Dial(ctx context.Context, id peer.ID) (inet.Conn, error) {
	req := NewDialRequest(ctx, id)

	// Step 1: invoke the preparers. Abort if any of them fails.
	if err := p.preparer.Prepare(req); err != nil {
		return req.Complete(nil, err)
	}

	// Preparers may complete the request.
	if req.IsComplete() {
		return req.Result()
	}

	// Step 2: resolve addresses for the peer.
	known, discovered, err := p.resolver.Resolve(req)
	if err != nil {
		err = xerrors.Errorf("error while resolving addresses for peer %s: %w", id.Pretty(), err)
		return req.Complete(nil, err)
	}

	if len(known) == 0 && discovered == nil {
		return req.Complete(nil, fmt.Errorf("no addresses to dial for peer %s", id.Pretty()))
	}

	// At this point we have a set of dialable addrs, and/or a channel where we'll receive new ones.
	var (
		success, errored []*Job

		inflight = make(map[*Job]struct{})
		planned  = make(chan []*Job, 16)
		resp     = make(chan *Job, 16)
	)

	plan, err := p.planner.NewPlan(req, known, planned)
	if err != nil {
		err = xerrors.Errorf("error while starting to plan the dial for peer %s: %w", id.Pretty(), err)
		return req.Complete(nil, err)
	}

	if discovered == nil {
		// There is no asynchronous address resolution occurring, so notify the plan immediately that we're done.
		plan.ResolutionDone()
	}

	// closure that tells us if we're done: when we have no inflight requests, and the planner has finished.
	// the addressresolver may still be working, but if the planner is satisfied with the result, so are we.
	done := func() bool { return len(inflight) == 0 && planned == nil }

	for {
		if done() {
			break
		}

		select {
		case job := <-resp:
			// Handle new responses coming in.
			if _, err := job.Result(); err == nil {
				success = append(success, job)
			} else {
				errored = append(errored, job)
			}
			delete(inflight, job)
			plan.JobComplete(job)

		case jobs, more := <-planned:
			// Push new planned jobs onto the throttler.
			if !more {
				log.Infof("finished planning dial jobs for peer: %s", id)
				if err := plan.Error(); err != nil {
					log.Errorf("planner failed for peer %s, failing request: %s", id, err)
					return req.Complete(nil, err)
				}
				planned = nil // stop receiving from this channel
				continue
			}
			for _, j := range jobs {
				j.SetResponseChan(resp)
				inflight[j] = struct{}{}
				p.throttleCh <- j
			}

		case addrs, more := <-discovered:
			if !more {
				log.Infof("finished discovering addresses for peer %s", id)
				plan.ResolutionDone()
				discovered = nil // stop receiving from this channel
				continue
			}
			plan.NewAddresses(addrs)

		case <-ctx.Done():
			return req.Complete(nil, ctx.Err())
		}
	}

	if len(success) == 0 && len(errored) == 0 {
		// if we performed no dials; this could happen if we had no addresses in the peerstore, and the discovery
		// process gave up before the context deadline fired.
		return req.Complete(nil, errors.New("no dials performed"))
	}

	if len(success) == 0 {
		var err *multierror.Error
		for _, j := range errored {
			_, e := j.Result()
			err = multierror.Append(err, e)
		}
		return req.Complete(nil, xerrors.Errorf("no successful dials: %w", err))
	}

	selected, err := plan.Select(success)
	if err != nil {
		err = xerrors.Errorf("planner failed to select a connection (amongst %v): %w", success, err)
		return req.Complete(nil, err)
	}
	conn, err := selected.Result()
	if err != nil {
		err = xerrors.Errorf("connection selected by planner was errored (multiaddr: %s): %w", selected.addr, err)
		return req.Complete(nil, err)
	}
	return req.Complete(p.addConnFn(conn))
}

func (p *Pipeline) SetPreparer(pr Preparer) {
	p.preparer = pr
}

func (p *Pipeline) SetAddressResolver(ar AddressResolver) {
	p.resolver = ar
}

func (p *Pipeline) SetPlanner(pl Planner) {
	p.planner = pl
}

func (p *Pipeline) SetThrottler(t Throttler) {
	p.throttler = t
}

func (p *Pipeline) SetExecutor(ex Executor) {
	p.executor = ex
}

func (p *Pipeline) Preparer() Preparer {
	return p.preparer
}

func (p *Pipeline) Planner() Planner {
	return p.planner
}

func (p *Pipeline) Throttler() Throttler {
	return p.throttler
}

func (p *Pipeline) Executor() Executor {
	return p.executor
}
