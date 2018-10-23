package dial

import "errors"

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
