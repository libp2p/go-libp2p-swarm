package dial

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	inet "github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-peer"
	tpt "github.com/libp2p/go-libp2p-transport"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	requestInflight = iota
	requestCompleted

	jobInflight = iota
	jobFinished
)

var ErrDialRequestCompleted = errors.New("invalid dial request status")
var ErrDialJobInvalidStatus = errors.New("invalid dial job status")

// Request represents a request from consumer code to dial a peer.
// TODO: some of these fields need to be exported to enable dial components to be implemented outside this package.
type Request struct {
	lk     sync.RWMutex
	net    inet.Network
	status int32

	// ctx is the parent context covering the entire request.
	ctx context.Context

	// The request starts with the peer ID only.
	id peer.ID

	// Addresses to be populated by a Preparer.
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
	// guard against callbacks calling Complete().
	if atomic.LoadInt32(&req.status) == requestCompleted {
		return ErrDialRequestCompleted
	}

	req.lk.Lock()

	if !atomic.CompareAndSwapInt32(&req.status, requestInflight, requestCompleted) {
		req.lk.Unlock()
		return ErrDialRequestCompleted
	}

	// notify anybody who's waiting on us to complete -- will be closed after releasing the lock.
	defer close(req.notifyCh)
	defer req.lk.Unlock()

	// set the completed values.
	req.conn, req.err = conn, err

	// call the callbacks in reverse order as they were added.
	for i := len(req.callbacks); i > 0; i-- {
		req.callbacks[i-1]()
	}

	return nil
}

// CompleteFrom completes the request using the values from the other request.
func (req *Request) CompleteFrom(other *Request) {
	req.Complete(other.Values())
}

func (req *Request) IsComplete() bool {
	req.lk.RLock()
	defer req.lk.RUnlock()

	return atomic.LoadInt32(&req.status) == requestCompleted
}

// Values returns the connection and error fields from this request.
// Both return values may be nil or incoherent unless the request has completed (see IsComplete).
func (req *Request) Values() (inet.Conn, error) {
	req.lk.RLock()
	defer req.lk.RUnlock()

	return req.conn, req.err
}

// AddCallback adds a function that will be invoked when this request completes, either in success
// or in failure.
func (req *Request) AddCallback(cb func()) {
	req.lk.Lock()
	defer req.lk.Unlock()

	req.callbacks = append(req.callbacks, cb)
}

// Job represents a dial attempt to a single multiaddr, within the scope of a Request.
// It can hold its own context, and its own callbacks.
type Job struct {
	status int32
	req    *Request

	ctx  context.Context
	addr ma.Multiaddr

	callbacks []func()

	// Result.
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
