package dial

import (
	"github.com/libp2p/go-libp2p-core/peer"
	lgbl "github.com/libp2p/go-libp2p-loggables"
)

type validator struct {
	local peer.ID
}

var _ Preparer = (*validator)(nil)

// NewValidator returns a Preparer that performs a sanity check on the dial request,
// erroring if any of these are true:
// 	- the peer ID is badly-formed.
// 	- we are dialing to ourselves.
func NewValidator(local peer.ID) Preparer {
	return &validator{local}
}

func (v *validator) Prepare(req *Request) error {
	id := req.PeerID()

	var logdial = lgbl.Dial("swarm", v.local, id, nil, nil)
	if err := id.Validate(); err != nil {
		return err
	}

	if id == v.local {
		log.Event(req.ctx, "swarmDialSelf", logdial)
		return ErrDialToSelf
	}

	defer log.EventBegin(req.ctx, "swarmDialAttemptSync", id).Done()
	return nil
}
