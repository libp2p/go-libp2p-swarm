package dial

import (
	"sync"

	"github.com/libp2p/go-libp2p-core/transport"
	ma "github.com/multiformats/go-multiaddr"
)

type jobCallbackEntry struct {
	name string
	fn   func(*Job)
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

// Result returns the connection and error fields from this job.
// Both return values may be nil or incoherent unless the job has completed.
func (j *Job) Result() (transport.CapableConn, error) {
	j.lk.Lock()
	defer j.lk.Unlock()

	return j.result.tconn, j.result.err
}

// Request returns the Request this dial job is associated with.
func (j *Job) Request() *Request {
	return j.req
}

// Address returns the target multiaddr of this dial job.
func (j *Job) Address() ma.Multiaddr {
	return j.addr
}

// Status returns the status of this job.
func (j *Job) Status() Status {
	j.lk.RLock()
	defer j.lk.RUnlock()

	return j.status
}

// SetResponseChan sets the response channel for this job. It is used to return the job to the pipeline upon completion.
// Note: a response channel MUST be set before completing the job, otherwise we will panic.
func (j *Job) SetResponseChan(respCh chan<- *Job) {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.respCh = respCh
}

// Cancelled indicates whether this job has been cancelled.
func (j *Job) Cancelled() bool {
	return j.Context().Err() == nil && j.req.Context() == nil
}

// Cancel signals this job to be cancelled.
func (j *Job) Cancel() {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	// we don't need to Complete the job, it is the executor's responsibility to do so.
	j.FireCancels()
}

// MarkBlocked marks this job as blocked. This status is of interest to the Planner.
func (j *Job) MarkBlocked() {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.status = StatusBlocked
}

// MarkInflight resumes this job from a blocked state. This status is of interest to the Planner.
func (j *Job) MarkInflight() {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.status = StatusInflight
}

// AddCallback adds a callback function that will be invoked when this job completes,
// either successfully or in error. Callbacks are executed in the inverse order they are added.
func (j *Job) AddCallback(name string, cb func(*Job)) {
	j.lk.Lock()
	defer j.lk.Unlock()

	j.status.Assert(StatusInflight | StatusBlocked)
	j.callbacks = append(j.callbacks, jobCallbackEntry{internedCallbackName(name), cb})
}

// Complete fills in the result of this dial job. It can only be invoked once per job. After the first completion,
// further calls to Complete will panic.
//
// Completing a job does the following:
//   1. Saves the provided result values.
//   2. Fires callbacks in the reverse order they were added.
//   3. Fires cancel functions in the reverse order they were added.
//   4. Notifies the response channel.
func (j *Job) Complete(conn transport.CapableConn, err error) {
	j.lk.Lock()

	if j.respCh == nil {
		panic("no response channel set for job; cannot complete")
	}

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
