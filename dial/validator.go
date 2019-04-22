package dial

import (
	"errors"

	"github.com/libp2p/go-libp2p-peer"
)

import lgbl "github.com/libp2p/go-libp2p-loggables"

// ErrDialToSelf is returned if we attempt to dial our own peer
var ErrDialToSelf = errors.New("dial to self attempted")

type validator struct{
	local peer.ID
}

var _ Preparer = (*validator)(nil)

// NewValidator returns a Preparer that performs a sanity check on the dial request, by erroring if:
// * the peer ID is badly-formed.
// * we are dialing to ourselves.
func NewValidator(local peer.ID) Preparer {
	return &validator{local}
}

func (v *validator) Prepare(req *Request) error {
	id := req.PeerID()
	log.Debugf("[%s] swarm dialing peer [%s]", v.local, id)

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
