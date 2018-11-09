package dial

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoTransport is returned when we don't know a transport for the
// given multiaddr.
var ErrNoTransport = errors.New("no transport for protocol")

type executor struct {
	resolver TransportResolverFn
	predial  []func(*Job)

	localCloseCh chan struct{}
}

var _ Executor = (*executor)(nil)

func NewExecutor(resolver TransportResolverFn, predial ...func(*Job)) Executor {
	return &executor{
		resolver:     resolver,
		predial:      predial,
		localCloseCh: make(chan struct{}),
	}
}

func (e *executor) Start(ctx context.Context, dialCh <-chan *Job) {
	for {
		select {
		case j := <-dialCh:
			go e.processDial(j)
		case <-ctx.Done():
			return
		case <-e.localCloseCh:
			return
		}
	}
}

func (e *executor) Close() error {
	close(e.localCloseCh)
	return nil
}

func (e *executor) processDial(job *Job) {
	defer func() {
		job.completeCh <- job
	}()

	for _, pd := range e.predial {
		pd(job)
	}

	// TODO: check if cancelled

	addr, id := job.addr, job.req.id
	log.Debugf("%s swarm dialing %s %s", job.req.net.LocalPeer(), id, addr)

	tpt := e.resolver(addr)
	if tpt == nil {
		job.err = ErrNoTransport
		return
	}

	tconn, err := tpt.Dial(job.req.ctx, addr, id)
	if err != nil {
		err = fmt.Errorf("%s --> %s dial attempt failed: %s", job.req.net.LocalPeer(), id, job.err)
		job.Complete(tconn, err)
		return
	}

	// Trust the transport? Yeah... right.
	if tconn.RemotePeer() != id {
		tconn.Close()
		err = fmt.Errorf("BUG in transport %T: tried to dial %s, dialed %s", id, job.tconn.RemotePeer(), tpt)
		log.Error(err)
		job.Complete(nil, err)
		return
	}

	job.Complete(tconn, err)
}
