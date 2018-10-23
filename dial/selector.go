package dial

import (
	"errors"

	"github.com/libp2p/go-libp2p-transport"
)

type selectFirstSuccessful struct{}

var _ Selector = (*selectFirstSuccessful)(nil)

var ErrNoSuccessfulDials = errors.New("no successful dials")

func NewSelectFirstSuccessfulDial() Selector {
	return &selectFirstSuccessful{}
}

func (selectFirstSuccessful) Select(successful dialJobs) (tpt.Conn, error) {
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
