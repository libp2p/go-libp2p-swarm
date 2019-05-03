package dial

import (
	"io"

	ma "github.com/multiformats/go-multiaddr"
)

// A Preparer performs operations on a dial Request before it is sent forward. It prepares the dial by performing
// sanity checks, validation, de-duplication, back-off, etc.
type Preparer interface {

	// Prepare processes this dial Request. The implementation may complete a dial early, either in error or
	// in success, by calling the Complete() method on the Request.
	//
	// Returning an error indicates something was wrong with the Preparer itself, and results in failure of the
	// dial Request.
	Prepare(req *Request) error
}

// An AddressResolver takes a peer ID and synchronously returns:
//
//   1. the known addresses for that peer (normally sourced from a Peerstore), if any.
//   2. an optional channel where additional addresses will be sent asynchronously.
//
// To discover fresh addresses, the address resolver can leverage a mechanism such as a DHT, mDNS, rendezvous
// points, etc. The AddressResolver must never block, so asynchronous discovery work must take place in
// a separate goroutine.
//
// The channel returned need not be a buffered channel: the pipeline won't block.
//
// The address resolver must close the channel once the discovery process is complete and there are no more
// addresses to return.
//
// Likewise, the background discovery process must stop when the provided context is closed.
type AddressResolver interface {
	Resolve(req *Request) (known []ma.Multiaddr, more <-chan []ma.Multiaddr, err error)
}

// Planners are components that are responsible for prioritising, grouping and scheduling dial jobs for particular
// addresses belonging to a peer.
//
// The simplest form of a Planner is the "immediate planner", which blindly dials all addresses in the order they
// are received/discovered. Examples of more sophisticated planner behaviour includes prioritising direct dials over
// relay dials, delaying costly transports, or triggering dials via public addresses when private subnets fail.
//
// The interface is reactive, and deliberately makes no assumptions about the underlying processes.
type Planner interface {
	// NewPlan creates a new dialing plan for a peer. The pipeline supplies the Request and any initial addresses
	// we already know of. It also supplies the channel where new dials must be queued.
	//
	// NewPlan must return a struct conforming to the Plan interface. The implementation can track inflight dials,
	// cancel them via Job.Cancel(), and it can spawn goroutines to stagger dials in time. It may close the out
	// channel to signal it has finished planning.
	//
	// The lifetime of asynchronous processes should obey the request context.
	NewPlan(req *Request, initial []ma.Multiaddr, out chan<- []*Job) (Plan, error)
}

type Plan interface {
	// NewAddresses is called by the pipeline to notify the Plan that there are new addresses to consider
	// for dialing.
	NewAddresses(found []ma.Multiaddr)

	// JobComplete is called by the pipeline to notify the Plan that a previously requested dial has completed,
	// either in success or in error.
	JobComplete(completed *Job)

	// ResolutionDone is called when the AddressResolver has finished.
	ResolutionDone()

	// Select allows the planner to choose which successful connection to keep.
	Select(successful []*Job) (selected *Job)

	// Error returns any error the planner wants to report back to the pipeline after closing the out channel.
	Error() error
}

// A Throttler is a global, singleton component (one per pipeline) that oversees all dial jobs that are emitted
// by Plan instances. It acts like a sentinel, guarding for fair/judicious usage of resources.
//
// To fulfill its role, it can throttle jobs based on file descriptors, number of inflight dials per peer, network
// conditions, bandwidth, fail rate, etc.
type Throttler interface {
	io.Closer

	// Run starts the throttling process. It receives planned jobs via the incoming channel, and emits jobs to dial via
	// the released channel. Both channels are provided by the caller, who should abstain from closing either during
	// normal operation to avoid panics. To safely shut down a throttler, first call Close() and await return.
	Run(incoming <-chan *Job, released chan<- *Job)
}

// An Executor dispatches network dials. Jobs sent to an Executor have already been subjected to the
// throttler, so the Executor must make progress immediately. Completed dials (either in success or failure) are
// sent back to the pipeline via the response channel.
type Executor interface {
	io.Closer

	// Run starts the dispatching process, feeding its input via the incoming channel.
	Run(incoming <-chan *Job)
}
