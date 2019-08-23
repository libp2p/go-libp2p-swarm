package dial

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type requestCallbackEntry struct {
	name string
	fn   func(*Request)
}

// Request represents a request from consumer code to dial a peer, identified by its peer ID.
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

// NewDialRequest creates a new Request to dial the provided peer ID.s
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

// PeerID returns the peer ID this Request is associated with.
func (r *Request) PeerID() peer.ID {
	return r.id
}

// Status returns the status of this request.
func (r *Request) Status() Status {
	r.lk.RLock()
	defer r.lk.RUnlock()

	return r.status
}

// Await returns a channel that will be closed once this Request completes.
func (r *Request) Await() <-chan struct{} {
	return r.notify
}

// Complete fills in the result of this dial request. It can only be invoked once per request.
// After the first completion, further calls to Complete will panic.
//
// Completing a job does the following:
//   1. Saves the provided result values.
//   2. Fires callbacks in the reverse order they were added.
//   3. Fires cancel functions in the reverse order they were added.
//   4. Closes notifyCh (see Await) to notify waiters.
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

// IsComplete returns whether this request has completed.
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
// either successfully or in error. Callbacks are executed in the inverse order they are added.
func (r *Request) AddCallback(name string, cb func(*Request)) {
	r.lk.Lock()
	defer r.lk.Unlock()

	r.status.Assert(StatusInflight)
	r.callbacks = append(r.callbacks, requestCallbackEntry{internedCallbackName(name), cb})
}

// CreateJob creates a dial job associated with this Request.
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
