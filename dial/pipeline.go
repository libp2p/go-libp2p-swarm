package dial

import (
	"context"
	"sync"
	"sync/atomic"

	"errors"

	"github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-peer"
	"github.com/libp2p/go-libp2p-transport"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	requestInflight = iota
	requestFinished

	jobInflight = iota
	jobFinished
)

var ErrDialRequestInvalidStatus = errors.New("invalid dial request status")
var ErrDialJobInvalidStatus = errors.New("invalid dial job status")

// Request represents a request from consumer code to dial a peer.
// TODO: some of these fields need to be exported to enable dial components to be implemented outside this package.
type Request struct {
	net    inet.Network
	status int32

	// ctx is the parent context covering the entire request.
	// dial job preparers may stem child contexts from this ctx.
	ctx context.Context

	// The request starts with the peer ID only.
	id peer.ID

	// Addresses to be populated by RequestPreparers.
	addrs []ma.Multiaddr

	// Functions to call when the request completes, either successfully or in error.
	callbacks []func()

	// Results.
	conn inet.Conn
	err  error

	// notifyCh will be closed when this request completes.
	notifyCh chan struct{}
}

func NewDialRequest(ctx context.Context, net inet.Network, id peer.ID) *Request {
	return &Request{
		ctx:      ctx,
		net:      net,
		id:       id,
		notifyCh: make(chan struct{}),
		status:   requestInflight,
	}
}

// Complete fills in the result of the dial request. It can only be invoked once per Request.
// After the first completion, further calls to complete will fail with error.
//
// Complete invokes all callbacks associated with this Request, and closes the notifyCh.
func (req *Request) Complete(conn inet.Conn, err error) error {
	if !atomic.CompareAndSwapInt32(&req.status, requestInflight, requestFinished) {
		return ErrDialRequestInvalidStatus
	}

	req.conn, req.err = conn, err
	for i := len(req.callbacks); i > 0; i-- {
		req.callbacks[i-1]()
	}

	close(req.notifyCh)
	return nil
}

func (req *Request) IsComplete() bool {
	return atomic.LoadInt32(&req.status) == requestFinished
}

// Values returns the connection and error fields from this request.
// Both return values may be nil or incoherent unless the request has completed (see IsComplete).
func (req *Request) Values() (inet.Conn, error) {
	return req.conn, req.err
}

// AddCallback adds a function that will be invoked when this request completes, either in success
// or in failure.
func (req *Request) AddCallback(cb func()) {
	req.callbacks = append(req.callbacks, cb)
}

// Job represents a dial attempt to a single multiaddr. It is associated 1-* to a Request.
// It can carry its own context and its own callbacks.
type Job struct {
	status int32
	req    *Request

	ctx  context.Context
	addr ma.Multiaddr

	callbacks []func()

	// result.
	tconn tpt.Conn
	err   error

	// Channel where completed dials should be sent.
	completeCh chan *Job
}

func NewDialJob(ctx context.Context, req *Request, addr ma.Multiaddr) *Job {
	return &Job{req: req, ctx: ctx, addr: addr, status: jobInflight}
}

func (j *Job) Cancelled() bool {
	select {
	case <-j.req.ctx.Done():
		return true
	case <-j.ctx.Done():
		return true
	default:
		return false
	}
}

func (j *Job) AddCallback(cb func()) {
	j.callbacks = append(j.callbacks, cb)
}

func (j *Job) Complete(conn tpt.Conn, err error) error {
	if !atomic.CompareAndSwapInt32(&j.status, jobInflight, jobFinished) {
		return ErrDialJobInvalidStatus
	}

	j.tconn, j.err = conn, err
	for i := len(j.callbacks); i > 0; i-- {
		j.callbacks[i-1]()
	}

	j.completeCh <- j
	return nil
}

type dialJobs []*Job

func (djs *dialJobs) sift() (success dialJobs, failed dialJobs) {
	for _, dj := range *djs {
		if dj.err == nil {
			success = append(success, dj)
			continue
		}
		failed = append(failed, dj)
	}
	return success, failed
}

type AddConnFn func(tc tpt.Conn, dir inet.Direction) (inet.Conn, error)

type Pipeline struct {
	lk  sync.RWMutex
	ctx context.Context

	net inet.Network

	reqPreparers []RequestPreparer
	planner      Planner
	throttler    Throttler
	jobPreparers []JobPreparer
	executor     Executor
	selector     Selector

	throttleCh chan *Job
	dialCh     chan *Job

	addConnFn AddConnFn
}

func (p *Pipeline) Component(name string, comp interface{}) error {
	switch comp.(type) {
	case RequestPreparer:
		p.reqPreparers = append(p.reqPreparers, comp.(RequestPreparer))
	case Planner:
		p.planner = comp.(Planner)
	case Throttler:
		p.throttler = comp.(Throttler)
	case JobPreparer:
		p.jobPreparers = append(p.jobPreparers, comp.(JobPreparer))
	case Executor:
		p.executor = comp.(Executor)
	case Selector:
		p.selector = comp.(Selector)
	default:
		return errors.New("unrecognized pipeline component")
	}
	return nil
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
	for _, p := range p.reqPreparers {
		if p.Prepare(req); req.IsComplete() {
			return req.Values()
		}
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

PLAN_EXECUTE:
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
					break PLAN_EXECUTE
				}
			} else if inflight == 0 {
				break PLAN_EXECUTE
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
		conn, err = p.selector.Select(success)
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
