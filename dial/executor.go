package dial

import (
	"errors"
	"fmt"

	tpt "github.com/libp2p/go-libp2p-transport"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
)

// ErrNoTransport is returned when we don't know a transport for the given multiaddr.
var ErrNoTransport = errors.New("no transport for protocol")

type TransportResolverFn = func(a ma.Multiaddr) tpt.Transport

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

	addr, id := job.addr, job.req.id
	log.Debugf("swarm dialing peer %s at addr %s", id, addr)

	tpt := e.resolver(addr)
	if tpt == nil {
		_ = job.Complete(nil, ErrNoTransport)
		return
	}

	tconn, err := tpt.Dial(job.Context(), addr, id)
	if err != nil {
		err = xerrors.Errorf("dial attempt to %s failed: %w", id, err)
		if err := job.Complete(nil, err); err != nil {
			log.Errorf("error while completing dial job for peer %s: %s", err)
		}
		return
	}

	// Trust the transport? Yeah... right.
	if tconn.RemotePeer() != id {
		tconn.Close()
		err = fmt.Errorf("BUG in transport %T: tried to dial %s, dialed %s", id, tconn.RemotePeer(), tpt)
		log.Error(err)
		job.Complete(nil, err)
		return
	}

	job.Complete(tconn, err)
}
