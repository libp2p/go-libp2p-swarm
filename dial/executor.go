package dial

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/transport"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
)

type TransportResolverFn = func(a ma.Multiaddr) transport.Transport

type executor struct {
	resolver TransportResolverFn
	predial  []func(*Job)

	closeCh chan struct{}
}

var _ Executor = (*executor)(nil)

func NewExecutor(resolver TransportResolverFn, predial ...func(*Job)) Executor {
	return &executor{
		resolver: resolver,
		predial:  predial,
		closeCh:  make(chan struct{}),
	}
}

func (e *executor) Run(dialCh <-chan *Job) {
	for {
		select {
		case j, more := <-dialCh:
			if !more {
				return
			}
			go e.processDial(j)
		case <-e.closeCh:
			return
		}
	}
}

func (e *executor) Close() error {
	close(e.closeCh)
	return nil
}

func (e *executor) processDial(job *Job) {
	for _, pd := range e.predial {
		pd(job)
	}

	addr, id := job.Address(), job.Request().PeerID()
	job.Debugf("executing dial")

	tpt := e.resolver(addr)
	if tpt == nil {
		job.Complete(nil, ErrNoTransport)
		return
	}

	tconn, err := tpt.Dial(job.Context(), addr, id)
	if err != nil {
		err = xerrors.Errorf("dial attempt to %s failed: %w", id, err)
		job.Complete(nil, err)
		return
	}

	// Trust the transport? Yeah... right.
	if tconn.RemotePeer() != id {
		tconn.Close()
		err = fmt.Errorf("BUG in transport %T: tried to dial %s, dialed %s", id, tconn.RemotePeer(), tpt)
		job.Errorf("%s", err)
		job.Complete(nil, err)
		return
	}

	job.Debugf("dial completed successfully")
	job.Complete(tconn, nil)
}
