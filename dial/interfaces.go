package dial

import (
	"context"
	"io"

	tpt "github.com/libp2p/go-libp2p-transport"
	ma "github.com/multiformats/go-multiaddr"
)

// A preparer can perform operations on a dial Request before it is sent to the Planner.
// Examples include validation, de-duplication, back-off, address resolution, etc.
//
// A Preparer may cancel the dial preemptively in error or in success, by calling Complete() on the Request.
type Preparer interface {
	Prepare(req *Request)
}

// Dial planners take a Request (populated with multiaddrs) and emit dial jobs on dialCh for the addresses
// they want dialed. The pipeline will call the planner once, as well as every time a dial job completes.
//
// For more information on the choreography, read the docs on Next().
type Planner interface {

	// Next requests the planner to send a new dialJobs on dialCh, if appropriate.
	//
	// When planning starts, Next is invoked with a nil last parameter.
	//
	// Next is then subsequently invoked on every completed dial, providing a slice of dialed jobs and the
	// last job to complete. With these two elements, in conjunction with any state that may be tracked, the Planner
	// can take decisions about what to dial next, or to finish planning altogether.
	//
	// When the planner is satisfied and has no more dials to request, it must signal so by closing
	// the dialCh channel.
	Next(req *Request, dialed dialJobs, last *Job, dialCh chan dialJobs) error
}

// A throttler is a goroutine that applies a throttling process to dial jobs requested by the Planner.
type Throttler interface {
	io.Closer

	// Start spawns the goroutine that is in charge of throttling. It receives planned jobs via inCh and emits
	// jobs to execute on dialCh. The throttler can apply any logic in-between: it may throttle jobs based on
	// system resources, time, inflight dials, network conditions, fail rate, etc.
	Start(ctx context.Context, inCh <-chan *Job, dialCh chan<- *Job)
}

// An executor is a goroutine responsible for ultimately carrying out the network dial to an addr.
type Executor interface {
	io.Closer

	// Start spawns the gorutine responsible for executing network dials. Jobs sent to dialCh have already
	// been subjected to the throttler. Once a dial finishes, the Executor must send the completed job to
	// completeCh, where it'll be received by the pipeline.
	//
	// In terms of concurrency, the Executor should behave like a dispatcher, in turn spawning individual
	// goroutines, or maintaining a finite set of child workers, to carry out the dials.
	// The Executor must never block.
	Start(ctx context.Context, dialCh <-chan *Job)
}

// Once the Planner is satisfied with the result of the dials, and all inflight dials have finished executing,
// the Selector picks the optimal successful connection to return to the consumer. The Pipeline takes care of
// closing unselected successful connections.
type Selector interface {
	Select(successful dialJobs) (tpt.Conn, error)
}

type TransportResolverFn func(a ma.Multiaddr) tpt.Transport
