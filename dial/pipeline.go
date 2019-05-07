package dial

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"
	"golang.org/x/xerrors"
)

type AddConnFn func(tc transport.CapableConn) (network.Conn, error)

var (
	// ErrDialBackoff is returned by the backoff code when a given peer has
	// been dialed too frequently
	ErrDialBackoff = errors.New("dial backoff")

	// ErrDialToSelf is returned if we attempt to dial our own peer
	ErrDialToSelf = errors.New("dial to self attempted")

	// ErrNoTransport is returned when we don't know a transport for the
	// given multiaddr.
	ErrNoTransport = errors.New("no transport for protocol")

	// ErrAllDialsFailed is returned when connecting to a peer has ultimately failed
	ErrAllDialsFailed = errors.New("all dials failed")

	// ErrNoAddresses is returned when we fail to find any addresses for a
	// peer we're trying to dial, or all addresses are unworkable.
	ErrNoAddresses = errors.New("no (good) addresses")

	// ErrPipelineStopped is returned when an action that requires a running pipeline is executed
	// on a stopped pipeline.
	ErrPipelineStopped = errors.New("the pipeline is stopped")

	// ErrPipelineRunning is returned when an action that requires a stopped pipeline is executed
	// on a running pipeline.
	ErrPipelineRunning = errors.New("the pipeline is running")
)

// Pipeline is the heart of the dialer. It represents a sequence of atomical components wired together, each with a
// clearly-delimited responsibility. The pipeline orchestrates these components to implement the dialing functionality.
//
// The Pipeline has five components. We provide brief descriptions below. For additional info, refer to the godoc
// of the appropriate interfaces:
//
//   - Preparer: runs preparatory actions prior to the dial execution. Actions may include: deduplicating, populating
//     timeouts, circuit breaking, validating the peer ID, etc.
//   - AddressResolver: populates the set of addresses for a peer, either from the peerstore and/or from other sources.
//   - Planner: schedules dials in time, responding to events from the environment, such as new addresses discovered,
//     or dial jobs completing.
//   - Throttler: throttles dials based on resource usage or other factors.
//   - Executor: actually carries out the network dials.
type Pipeline struct {
	lk      sync.RWMutex
	ctx     context.Context
	running bool

	network network.Network

	preparer  Preparer
	resolver  AddressResolver
	planner   Planner
	throttler Throttler
	executor  Executor

	throttleCh chan *Job
	executeCh  chan *Job

	addConnFn AddConnFn
}

func NewPipeline(ctx context.Context, net network.Network, addConnFn AddConnFn) *Pipeline {
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
	p.lk.Lock()
	defer p.lk.Unlock()

	if p.running {
		return
	}

	go p.executor.Run(p.executeCh)
	go p.throttler.Run(p.throttleCh, p.executeCh)
	p.running = true
}

func (p *Pipeline) Close() error {
	p.lk.Lock()
	defer p.lk.Unlock()

	if !p.running {
		return nil
	}

	var errs []error
	if err := p.throttler.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := p.executor.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while closing the dial pipeline: %v", errs)
	}
	p.running = false
	return nil
}

func (p *Pipeline) Dial(ctx context.Context, id peer.ID) (network.Conn, error) {
	p.lk.RLock()
	if !p.running {
		return nil, ErrPipelineStopped
	}
	p.lk.RUnlock()

	req := NewDialRequest(ctx, id)
	req.Debugf("starting to dial peer")

	// Invoke the preparers. Abort if any of them fails.
	if err := p.preparer.Prepare(req); err != nil {
		req.Debugf("dial completed in error by a preparer: %s", err)
		return req.Complete(nil, err)
	}

	// Preparers may complete the request.
	if req.IsComplete() {
		req.Debugf("dial completed early by a preparer")
		return req.Result()
	}

	// Resolve addresses for the peer.
	known, discovered, err := p.resolver.Resolve(req)
	if err != nil {
		req.Warningf("error while resolving addresses: %s", err)
		err = xerrors.Errorf("error while resolving addresses: %w", err)
		return req.Complete(nil, err)
	}

	if len(known) == 0 && discovered == nil {
		req.Debugf("no known addresses nor discovery process; aborting dial")
		return req.Complete(nil, ErrNoAddresses)
	}

	// At this point we have a set of dialable addrs, and/or a channel where we'll receive new ones.
	var (
		success   []*Job
		inflight  = make(map[*Job]struct{})
		planned   = make(chan []*Job, 16)
		resp      = make(chan *Job, 16)
		attempted = 0
	)

	plan, err := p.planner.NewPlan(req, known, planned)
	if err != nil {
		req.Warningf("error while starting to plan the dial; aborting dial")
		err = xerrors.Errorf("error while starting to plan the dial: %w", err)
		return req.Complete(nil, err)
	}

	if discovered == nil {
		// There is no asynchronous address resolution occurring, so notify the plan immediately that we're done.
		plan.ResolutionDone()
	}

	// closure that tells us if we're done: when we have no inflight requests, and the planner has finished.
	// the addressresolver may still be working, but if the planner is satisfied with the result, so are we.
	done := func() bool { return len(inflight) == 0 && planned == nil }
	dialErr := new(Error)

	for {
		if done() {
			close(resp)
			break
		}

		select {
		case job := <-resp:
			req.Debugf("job completed")
			// Handle new responses coming in.
			if _, err := job.Result(); err == nil {
				success = append(success, job)
			} else {
				dialErr.recordErr(job.Address(), err)
			}
			delete(inflight, job)
			plan.JobComplete(job)

		case jobs, more := <-planned:
			// Push new planned jobs onto the throttler.
			if !more {
				req.Debugf("finished planning dial jobs")
				if err := plan.Error(); err != nil {
					req.Errorf("planner failed with error: %s", err)
					return req.Complete(nil, err)
				}
				planned = nil // stop receiving from this channel
				continue
			}
			for _, j := range jobs {
				j.Debugf("job planned")
				j.SetResponseChan(resp)
				inflight[j] = struct{}{}
				attempted++
				p.throttleCh <- j
			}

		case addrs, more := <-discovered:
			if !more {
				req.Debugf("finished discovering addresses")
				plan.ResolutionDone()
				discovered = nil // stop receiving from this channel
				continue
			}
			req.Debugf("discovered addresses: %v", addrs)
			plan.NewAddresses(addrs)

		case <-ctx.Done():
			req.Debugf("context completed")
			return req.Complete(nil, ctx.Err())
		}
	}

	if len(success) == 0 {
		if attempted == 0 {
			// we performed no dials; this could happen if we had no addresses in the peerstore,
			// and the discovery process gave up before the context deadline fired.
			req.Debugf("failed with no remote addresses")
			dialErr.Cause = network.ErrNoRemoteAddrs
		} else {
			req.Debugf("failed because all attempted dials failed")
			dialErr.Cause = ErrAllDialsFailed
		}
		return req.Complete(nil, dialErr)
	}

	var selected *Job
	if selected = plan.Select(success); selected == nil {
		panic(fmt.Sprintf("planner failed to select a connection amongst %v", success))
	}

	var conn transport.CapableConn
	if conn, err = selected.Result(); err != nil {
		panic(fmt.Sprintf("connection selected by planner was errored (multiaddr: %s): %s", selected.Address(), err))
	}

	// close unselected connections.
	for _, j := range success {
		if j == selected {
			continue
		}
		if conn, _ := j.Result(); conn != nil {
			_ = conn.Close()
		}
	}

	req.Debugf("dial completed successfully")
	return req.Complete(p.addConnFn(conn))
}

func (p *Pipeline) SetPreparer(pr Preparer) error {
	p.lk.RLock()
	defer p.lk.RUnlock()

	if p.running {
		return ErrPipelineRunning
	}

	p.preparer = pr
	return nil
}

func (p *Pipeline) SetAddressResolver(ar AddressResolver) error {
	p.lk.RLock()
	defer p.lk.RUnlock()

	if p.running {
		return ErrPipelineRunning
	}

	p.resolver = ar
	return nil
}

func (p *Pipeline) SetPlanner(pl Planner) error {
	p.lk.RLock()
	defer p.lk.RUnlock()

	if p.running {
		return ErrPipelineRunning
	}

	p.planner = pl
	return nil
}

func (p *Pipeline) SetThrottler(t Throttler) error {
	p.lk.RLock()
	defer p.lk.RUnlock()

	if p.running {
		return ErrPipelineRunning
	}

	p.throttler = t
	return nil
}

func (p *Pipeline) SetExecutor(ex Executor) error {
	p.lk.RLock()
	defer p.lk.RUnlock()

	if p.running {
		return ErrPipelineRunning
	}

	p.executor = ex
	return nil
}

func (p *Pipeline) Preparer() Preparer {
	return p.preparer
}

func (p *Pipeline) AddressResolver() AddressResolver {
	return p.resolver
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
