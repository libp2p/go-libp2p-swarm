package dial

import (
	"context"
	"errors"
	"fmt"
	"sync"

	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	tpt "github.com/libp2p/go-libp2p-transport"
)

type AddConnFn func(tc tpt.Conn, dir inet.Direction) (inet.Conn, error)

// The Pipeline is the central structure of the dialer. One could think of it as the controller or coordinator.
//
// It is comprised by a chain of components that are wired together, following the _pipeline_ design pattern
// in traditional software architecture.
//
// TODO: more docs.
type Pipeline struct {
	lk  sync.RWMutex
	ctx context.Context

	net inet.Network

	preparer  Preparer
	planner   Planner
	throttler Throttler
	executor  Executor

	throttleCh chan *Job
	dialCh     chan *Job

	addConnFn AddConnFn
}

func (p *Pipeline) SetPreparer(pr Preparer) {
	p.preparer = pr
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

func NewPipeline(ctx context.Context, net inet.Network, addConnFn AddConnFn) *Pipeline {
	pipeline := &Pipeline{
		ctx:       ctx,
		net:       net,
		addConnFn: addConnFn,
	}
	return pipeline
}

func (p *Pipeline) Start(ctx context.Context) {
	p.dialCh = make(chan *Job, 100)
	p.throttleCh = make(chan *Job, 100)

	go p.executor.Start(ctx, p.dialCh)
	go p.throttler.Start(ctx, p.throttleCh, p.dialCh)
}

func (p *Pipeline) Dial(ctx context.Context, id peer.ID) (inet.Conn, error) {
	req := NewDialRequest(ctx, p.net, id)

	// Prepare the dial.
	if p.preparer.Prepare(req); req.IsComplete() {
		return req.Values()
	}

	if len(req.addrs) == 0 {
		return nil, errors.New("no addresses to dial")
	}

	// At this point we have a set of dialable maddrs.
	var (
		conn   tpt.Conn
		err    error
		dialed dialJobs
		planCh = make(chan dialJobs, 1)
		respCh = make(chan *Job)
	)

	if err := p.planner.Next(req, dialed, nil, planCh); err != nil {
		req.Complete(nil, err)
		return req.Values()
	}

	// no need to synchronize access to inflight, as it's locally bound and single-threaded.
	inflight := 0

PlanExecute:
	for {
		select {
		case jobs, more := <-planCh:
			inflight = inflight + len(jobs)
			for _, j := range jobs {
				j.completeCh = respCh
				dialed = append(dialed, j)
				p.throttleCh <- j
			}
			if !more {
				// stop reading from this channel
				planCh = nil
			}

		case res := <-respCh:
			inflight--
			if planCh != nil {
				err := p.planner.Next(req, dialed, res, planCh)
				if err != nil {
					req.Complete(nil, err)
					break PlanExecute
				}
			} else if inflight == 0 {
				break PlanExecute
			}

		case <-ctx.Done():
			req.Complete(nil, ctx.Err())
			return req.Values()
		}
	}

	success, _ := dialed.sift()
	switch len(success) {
	case 0:
		req.Complete(nil, errors.New("no successful dials"))
	case 1:
		sconn, err := p.addConnFn(success[0].tconn, inet.DirOutbound)
		req.Complete(sconn, err)
	default:
		conn, err = p.planner.Select(success)
		if err != nil {
			req.Complete(nil, errors.New("failed while selecting a connection"))
			break
		}
		sconn, err := p.addConnFn(conn, inet.DirOutbound)
		req.Complete(sconn, err)

		// close connections that were not selected
		for _, s := range success {
			if s.tconn != conn {
				if err := s.tconn.Close(); err != nil {
					// TODO log error while closing unselected connection
				}
			}
		}
	}

	// Callbacks could modify the result, so return the values from the context, instead of local vars.
	return req.Values()
}

type preparerBinding struct {
	name string
	p    Preparer
}

// PreparerSeq is a Preparer that daisy-chains the Request through an ordered list of preparers. It short-circuits
// the process if a Preparer completes the Request. Preparers are bound by unique names.
type PreparerSeq struct {
	lk  sync.Mutex
	seq []preparerBinding
}

var _ Preparer = (*PreparerSeq)(nil)

func (ps *PreparerSeq) Prepare(req *Request) {
	for _, p := range ps.seq {
		if p.p.Prepare(req); req.IsComplete() {
			break
		}
	}
}

func (ps *PreparerSeq) find(name string) (i int, res *preparerBinding) {
	for i, pb := range ps.seq {
		if pb.name == name {
			return i, &pb
		}
	}
	return -1, nil
}

func (ps *PreparerSeq) AddFirst(name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, prev := ps.find(name); prev != nil {
		return fmt.Errorf("a preparer with name %s already exists", name)
	}
	pb := preparerBinding{name, preparer}
	ps.seq = append([]preparerBinding{pb}, ps.seq...)
	return nil
}

func (ps *PreparerSeq) AddLast(name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, prev := ps.find(name); prev != nil {
		return fmt.Errorf("a preparer with name %s already exists", name)
	}
	pb := preparerBinding{name, preparer}
	ps.seq = append(ps.seq, pb)
	return nil
}

func (ps *PreparerSeq) InsertBefore(before, name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	i, prev := ps.find(before)
	if prev == nil {
		return fmt.Errorf("no preparers found with name %s", name)
	}

	pb := preparerBinding{name, preparer}
	ps.seq = append(ps.seq, pb)
	copy(ps.seq[i+1:], ps.seq[i:])
	ps.seq[i] = pb

	return nil
}

func (ps *PreparerSeq) InsertAfter(after, name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	i, prev := ps.find(after)
	if prev == nil {
		return fmt.Errorf("no preparers found with name %s", name)
	}

	pb := preparerBinding{name, preparer}
	ps.seq = append(ps.seq, pb)
	copy(ps.seq[i+2:], ps.seq[i+1:])
	ps.seq[i+1] = pb

	return nil
}

func (ps *PreparerSeq) Replace(old, name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	i, prev := ps.find(old)
	if prev == nil {
		return fmt.Errorf("no preparers found with name %s", name)
	}
	ps.seq[i] = preparerBinding{name, preparer}
	return nil
}

func (ps *PreparerSeq) Remove(name string) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if i, prev := ps.find(name); prev != nil {
		ps.seq = append(ps.seq[:i], ps.seq[i+1:]...)
	}
}

func (ps *PreparerSeq) Get(name string) Preparer {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, pb := ps.find(name); pb != nil {
		return pb.p
	}
	return nil
}
