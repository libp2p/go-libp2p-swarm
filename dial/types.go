package dial

import (
	"context"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"
	ma "github.com/multiformats/go-multiaddr"
)

type Status uint32

const (
	StatusInflight Status = 1 << iota
	StatusBlocked
	StatusCompleting
	StatusComplete
)

func (s *Status) Assert(mask Status) {
	if *s&mask == 0 {
		// it may be worth decoding the mask to a friendlier format.
		panic(fmt.Sprintf("illegal state %s; mask: %b", s, mask))
	}
}

func (s Status) String() string {
	switch s {
	case StatusComplete:
		return "Status(Complete)"
	case StatusInflight:
		return "Status(Inflight)"
	case StatusCompleting:
		return "Status(Completing)"
	case StatusBlocked:
		return "Status(Blocked)"
	default:
		return fmt.Sprintf("Status(%d)", s)
	}
}

var internCallbacks = make(map[string]string)

func internedCallbackName(name string) string {
	if n, ok := internCallbacks[name]; ok {
		return n
	}
	internCallbacks[name] = name
	return name
}

type (
	RequestCallback = func(*Request)
	JobCallback     = func(*Job)

	requestCallbackEntry struct {
		name string
		fn   RequestCallback
	}

	jobCallbackEntry struct {
		name string
		fn   JobCallback
	}
)

type contextHolder struct {
	clk     sync.RWMutex
	ctx     context.Context
	cancels []context.CancelFunc
}

func (ch *contextHolder) MutateContext(mutator func(orig context.Context) (context.Context, context.CancelFunc)) {
	ch.clk.Lock()
	defer ch.clk.Unlock()

	ctx, cancel := mutator(ch.ctx)
	ch.ctx = ctx
	ch.cancels = append(ch.cancels, cancel)
}

func (ch *contextHolder) Context() context.Context {
	ch.clk.RLock()
	defer ch.clk.RUnlock()

	return ch.ctx
}

func (ch *contextHolder) FireCancels() {
	ch.clk.Lock()
	defer ch.clk.Unlock()

	for i := len(ch.cancels) - 1; i >= 0; i-- {
		ch.cancels[i]()
	}
}

// Request represents a request from consumer code to dial a peer.
type Request struct {
	*contextHolder
	id     peer.ID       // peer ID we're dialing.
	notify chan struct{} // closed when this request completes.

	lk     sync.RWMutex
	status Status

	cblk      sync.RWMutex
	callbacks []requestCallbackEntry

	result struct {
		conn network.Conn
		err  error
	}
}

func NewDialRequest(ctx context.Context, id peer.ID) *Request {
	// by creating a cancellable context we control, we can stop request-scoped asynchronous processes,
	// like the planner, without relying on the consumer correctly cancelling the passed-in context.
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	req := &Request{
		contextHolder: &contextHolder{ctx: ctx, cancels: []context.CancelFunc{cancel}},
		id:            id,
		notify:        make(chan struct{}),
		status:        StatusInflight,
	}
	return req
}

func (r *Request) PeerID() peer.ID {
	return r.id
}

func (r *Request) Status() Status {
	r.lk.RLock()
	defer r.lk.RUnlock()

	return r.status
}

func (r *Request) Await() <-chan struct{} {
	return r.notify
}

// Complete fills in the result of the dial request. It can only be invoked once per Request.
// After the first completion, further calls to complete will panic.
//
// Complete invokes all callbacks associated with this Request, and closes the notifyCh.
func (r *Request) Complete(conn network.Conn, err error) (network.Conn, error) {
	r.lk.Lock()

	r.status.Assert(StatusInflight)
	r.status = StatusCompleting

	r.result.conn, r.result.err = conn, err

	// drop the lock so that callbacks can access our fields.
	// there's no concurrency risk because the status already guards against double completes.
	r.lk.Unlock()

	// call the callbacks in reverse order as they were added.
	for i := len(r.callbacks) - 1; i >= 0; i-- {
		cb := r.callbacks[i]
		log.Debugf("triggering request callback for peer %s: %s", r.id, cb.name)
		cb.fn(r)
	}

	// notify anybody who's waiting on us to complete -- after the lock is released.
	defer close(r.notify)

	r.lk.Lock()
	defer r.lk.Unlock()

	r.status.Assert(StatusCompleting)
	r.status = StatusComplete

	// by cancelling our context explicitly we are not vulnerable to incorrect behaviour by
	// consumers that do not cancel the passed in context.
	r.FireCancels()

	// note: callbacks may have modified the results.
	return r.result.conn, r.result.err
}

func (r *Request) IsComplete() bool {
	r.lk.RLock()
	defer r.lk.RUnlock()

	return r.status == StatusComplete
}

// CompleteFrom completes the request using the result from another request.
func (r *Request) CompleteFrom(other *Request) (network.Conn, error) {
	return r.Complete(other.Result())
}

// Result returns the connection and error fields from this request.
// Both return values may be nil or incoherent unless the request has completed (see IsComplete).
func (r *Request) Result() (network.Conn, error) {
	r.lk.RLock()
	defer r.lk.RUnlock()

	return r.result.conn, r.result.err
}

// AddCallback adds a callback function that will be invoked when this request completes,
// either successfully or in error.
func (r *Request) AddCallback(name string, cb RequestCallback) {
	r.lk.Lock()
	defer r.lk.Unlock()

	r.status.Assert(StatusInflight)
	r.callbacks = append(r.callbacks, requestCallbackEntry{internedCallbackName(name), cb})
}

func (r *Request) CreateJob(addr ma.Multiaddr) *Job {
	r.lk.RLock()
	defer r.lk.RUnlock()

	r.status.Assert(StatusInflight)

	log.Debugf("creating job for peer %s for addr %s", r.id, addr)
	ctx, cancel := context.WithCancel(r.ctx)
	job := &Job{
		contextHolder: &contextHolder{ctx: ctx, cancels: []context.CancelFunc{cancel}},
		req:           r,
		addr:          addr,
		status:        StatusInflight,
	}
	return job
}

// Job represents a dial job to a concrete multiaddr within the scope of a Request.
type Job struct {
	*contextHolder

	req  *Request
	addr ma.Multiaddr

	lk        sync.RWMutex
	status    Status
	respCh    chan<- *Job
	callbacks []jobCallbackEntry
	result    struct {
		tconn transport.CapableConn
		err   error
	}
}

func (j *Job) Result() (transport.CapableConn, error) {
	j.lk.Lock()
	defer j.lk.Unlock()

	return j.result.tconn, j.result.err
}

func (j *Job) Request() *Request {
	return j.req
}

func (j *Job) Address() ma.Multiaddr {
	return j.addr
}

func (j *Job) Status() Status {
	j.lk.RLock()
	defer j.lk.RUnlock()

	return j.status
}

func (j *Job) SetResponseChan(respCh chan<- *Job) {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.respCh = respCh
}

func (j *Job) Cancelled() bool {
	return j.Context().Err() == nil && j.req.Context() == nil
}

func (j *Job) Cancel() {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.FireCancels()
}

func (j *Job) MarkBlocked() {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.status = StatusBlocked
}

func (j *Job) MarkInflight() {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.status = StatusInflight
}

func (j *Job) AddCallback(name string, cb JobCallback) {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.callbacks = append(j.callbacks, jobCallbackEntry{internedCallbackName(name), cb})
}

func (j *Job) Complete(conn transport.CapableConn, err error) {
	j.lk.Lock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.status = StatusCompleting

	j.result.tconn, j.result.err = conn, err

	// drop the lock so that callbacks can access our fields.
	// there's no concurrency risk because the status already guards against double completes.
	j.lk.Unlock()

	for i := len(j.callbacks) - 1; i >= 0; i-- {
		cb := j.callbacks[i]
		log.Debugf("triggering job callback for peer %s: %s", j.req.id, cb.name)
		cb.fn(j)
	}

	j.lk.Lock()
	j.status = StatusComplete
	j.lk.Unlock()

	j.FireCancels()

	select {
	case j.respCh <- j:
	case <-j.req.Context().Done():
	default:
		// response channel is backlogged; trigger an ephemeral goroutine to avoid blocking
		// this should not happen often, but when it does, we assume the cost.
		log.Warningf("response chan for dial jobs for peer %s is backlogged; "+
			"spawning goroutine to avoid blocking", j.req.id)
		go func(req *Request) {
			select {
			case j.respCh <- j:
			case <-req.Context().Done():
			}
		}(j.req)
	}
}
