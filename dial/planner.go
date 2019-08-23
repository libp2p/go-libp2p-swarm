package dial

import (
	"fmt"

	ma "github.com/multiformats/go-multiaddr"
)

var _ Planner = (*immediatePlanner)(nil)
var _ Plan = (*immediatePlan)(nil)

type immediatePlanner struct{}

// NewImmediatePlanner returns a planner that plans both initial and discovered addresses immediately.
func NewImmediatePlanner() Planner {
	return &immediatePlanner{}
}

func (*immediatePlanner) NewPlan(req *Request, initial []ma.Multiaddr, out chan<- []*Job) (Plan, error) {
	plan := &immediatePlan{req, out, nil}
	plan.NewAddresses(initial)
	return plan, nil
}

type immediatePlan struct {
	req *Request
	out chan<- []*Job
	err error
}

func (ip *immediatePlan) NewAddresses(found []ma.Multiaddr) {
	jobs := make([]*Job, 0, len(found))
	for _, addr := range found {
		jobs = append(jobs, ip.req.CreateJob(addr))
	}

	select {
	case ip.out <- jobs:
		// all ok, we were able to send the jobs.
	default:
		// channel is backlogged, so let's schedule a goroutine to do the send.
		go func() {
			select {
			case ip.out <- jobs:
			case <-ip.req.ctx.Done():
			}
		}()
	}
}

func (ip *immediatePlan) JobComplete(job *Job) {
	// noop.
}

func (ip *immediatePlan) ResolutionDone() {
	// close the out channel to signal we have no more jobs to plan.
	close(ip.out)
}

func (ip *immediatePlan) Select(successful []*Job) *Job {
	if len(successful) == 0 {
		panic(fmt.Sprintf("planner was asked to select amongst no successful connections: %v", successful))
	}
	return successful[0]
}

func (ip *immediatePlan) Error() error {
	return ip.err
}
