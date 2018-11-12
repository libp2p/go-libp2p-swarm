package dial

import (
	"errors"

	"github.com/libp2p/go-libp2p-transport"
)

// ErrNoSuccessfulDials is returned by Select() when all dials failed.
var ErrNoSuccessfulDials = errors.New("no successful dials")

type singleBurstPlanner struct{}

var _ Planner = (*singleBurstPlanner)(nil)

func NewSingleBurstPlanner() Planner {
	return &singleBurstPlanner{}
}

func (*singleBurstPlanner) Next(req *Request, dialled dialJobs, last *Job, dialCh chan dialJobs) error {
	if last != nil {
		return errors.New("unexpected call to planner")
	}

	var jobs dialJobs
	for _, maddr := range req.addrs {
		jobs = append(jobs, NewDialJob(req.ctx, req, maddr))
	}

	dialCh <- jobs
	close(dialCh)

	return nil
}

func (*singleBurstPlanner) Select(successful dialJobs) (tpt.Conn, error) {
	if len(successful) == 0 {
		return nil, ErrNoSuccessfulDials
	}

	// return the first successful dial.
	for _, j := range successful {
		if j.err == nil && j.tconn != nil {
			return j.tconn, nil
		}
	}

	return nil, ErrNoSuccessfulDials
}
