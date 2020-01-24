package swarm

import (
	"github.com/libp2p/go-libp2p-core/introspect"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/pkg/errors"
)

func (swarm *Swarm) toIntrospectConnectionsPb(query introspect.ConnectionQueryInput) ([]*introspect.Connection, error) {
	swarm.conns.RLock()
	defer swarm.conns.RUnlock()

	containsId := func(ids []introspect.ConnID, id string) bool {
		for _, i := range ids {
			if string(i) == id {
				return true
			}
		}
		return false
	}

	var iconns []*introspect.Connection

	for _, conns := range swarm.conns.m {
		for _, c := range conns {
			switch query.Type {
			case introspect.ConnListQueryTypeForIds:
				if containsId(query.ConnIDs, c.id) {
					ic, err := swarm.toIntrospectConnectionPb(query, c)
					if err != nil {
						return nil, errors.Wrap(err, "failed to convert connection to introspect.Connection")
					}
					iconns = append(iconns, ic)
				}
			case introspect.ConnListQueryTypeAll:
				ic, err := swarm.toIntrospectConnectionPb(query, c)
				if err != nil {
					return nil, errors.Wrap(err, "failed to convert connection to introspect.Connection")
				}
				iconns = append(iconns, ic)
			}
		}
	}
	return iconns, nil
}

func (swarm *Swarm) toIntrospectConnectionPb(query introspect.ConnectionQueryInput, c *Conn) (*introspect.Connection, error) {
	c.streams.Lock()
	defer c.streams.Unlock()

	ic := &introspect.Connection{}
	ic.Id = c.id
	ic.PeerId = c.RemotePeer().String()

	ic.Endpoints = &introspect.EndpointPair{SrcMultiaddr: c.LocalMultiaddr().String(), DstMultiaddr: c.RemoteMultiaddr().String()}

	switch c.stat.Direction {
	case network.DirInbound:
		ic.Role = introspect.Role_RESPONDER
	case network.DirOutbound:
		ic.Role = introspect.Role_INITIATOR
	}

	sl := &introspect.StreamList{}
	for s, _ := range c.streams.m {
		switch query.StreamOutputType {
		case introspect.QueryOutputTypeFull:
			isl, err := swarm.toIntrospectStreamPb(s, ic)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert swarm stream to introspect.Stream")
			}
			sl.Streams = append(sl.Streams, isl)
		case introspect.QueryOutputTypeIds:
			sl.StreamIds = append(sl.StreamIds, []byte(s.id))
		}
	}
	ic.Streams = sl

	// TODO How ?
	ic.Timeline = &introspect.Connection_Timeline{}

	// TODO How ?
	ic.Traffic = &introspect.Traffic{}

	// TODO How ?
	ic.Attribs = &introspect.Connection_Attributes{}

	// TODO How do we track other states ?
	ic.Status = introspect.Status_ACTIVE

	// TODO What's this ?
	ic.TransportId = nil

	// TODO What's this ?
	ic.UserProvidedTags = []string{""}

	// TODO How ?
	ic.LatencyNs = 0

	// TODO How ?
	//ic.RelayedOver

	return ic, nil
}

func (swarm *Swarm) toIntrospectStreamPb(s *Stream, ic *introspect.Connection) (*introspect.Stream, error) {
	is := &introspect.Stream{}
	is.Id = s.id
	is.Protocol = string(s.Protocol())

	switch s.stat.Direction {
	case network.DirInbound:
		is.Role = introspect.Role_RESPONDER
	case network.DirOutbound:
		is.Role = introspect.Role_INITIATOR
	}

	is.Conn = &introspect.Stream_ConnectionRef{Connection: &introspect.Stream_ConnectionRef_Conn{ic}}

	s.state.Lock()
	defer s.state.Unlock()
	switch s.state.v {
	case streamOpen:
		is.Status = introspect.Status_ACTIVE
	case streamReset:
		is.Status = introspect.Status_ERROR
	case streamCloseBoth:
		is.Status = introspect.Status_CLOSED
	default:
		is.Status = introspect.Status_ACTIVE
	}

	// TODO How ?
	is.Timeline = &introspect.Stream_Timeline{}

	// TODO How ?
	is.Traffic = &introspect.Traffic{}

	// TODO How ?
	is.LatencyNs = 0

	// TODO What's this ?
	is.UserProvidedTags = []string{""}

	return is, nil
}
