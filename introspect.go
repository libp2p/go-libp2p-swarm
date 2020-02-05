package swarm

import (
	"github.com/libp2p/go-libp2p-core/introspect"
	introspectpb "github.com/libp2p/go-libp2p-core/introspect/pb"
	"github.com/pkg/errors"
)

// IntrospectTraffic introspects & returns the overall traffic for this peer
func (s *Swarm) IntrospectTraffic() (*introspectpb.Traffic, error) {
	if s.bwc != nil {
		t := &introspectpb.Traffic{}
		metrics := s.bwc.GetBandwidthTotals()

		t.TrafficIn = &introspectpb.DataGauge{CumBytes: uint64(metrics.TotalIn), InstBw: uint64(metrics.RateIn)}
		t.TrafficOut = &introspectpb.DataGauge{CumBytes: uint64(metrics.TotalOut), InstBw: uint64(metrics.RateOut)}

		return t, nil
	}

	return nil, nil
}

// IntrospectConnections introspects & returns the swarm connections
func (swarm *Swarm) IntrospectConnections(query introspect.ConnectionQueryParams) ([]*introspectpb.Connection, error) {
	swarm.conns.RLock()
	defer swarm.conns.RUnlock()

	containsId := func(ids []introspect.ConnectionID, id string) bool {
		for _, i := range ids {
			if string(i) == id {
				return true
			}
		}
		return false
	}

	var iconns []*introspectpb.Connection

	appendConnection := func(c *Conn) error {
		if query.Output == introspect.QueryOutputFull {
			ic, err := swarm.introspectConnection(c)
			if err != nil {
				return errors.Wrap(err, "failed to convert connection to introspect.Connection")
			}
			iconns = append(iconns, ic)
		} else {
			iconns = append(iconns, &introspectpb.Connection{Id: c.id})
		}
		return nil
	}

	// iterate over all connections the swarm has & resolve the ones required by the query
	for _, conns := range swarm.conns.m {
		for _, c := range conns {
			if query.Include != nil {
				if containsId(query.Include, c.id) {
					if err := appendConnection(c); err != nil {
						return nil, err
					}
				}
			} else {
				if err := appendConnection(c); err != nil {
					return nil, err
				}
			}
		}
	}
	return iconns, nil
}

func (swarm *Swarm) introspectConnection(c *Conn) (*introspectpb.Connection, error) {
	c.streams.Lock()
	defer c.streams.Unlock()

	ci := &connIntrospector{c, swarm}
	ic := &introspectpb.Connection{}

	ic.Id = ci.id()
	ic.PeerId = ci.remotePeerId()
	ic.Endpoints = ci.endPoints()
	ic.Role = ci.role()

	streams, err := ci.streams()
	if err != nil {
		return nil, errors.Wrap(err, "failed to introspect stream for connection")
	}
	ic.Streams = streams

	ic.Traffic = ci.traffic()
	ic.Timeline = ci.timeline()
	ic.Attribs = ci.attribs()
	ic.Status = ci.status()
	ic.TransportId = ci.transportId()
	ic.UserProvidedTags = ci.userProvidedTags()
	ic.LatencyNs = ci.latencyNs()
	ic.RelayedOver = ci.relayedOver()

	return ic, nil

}

func (swarm *Swarm) IntrospectStreams(query introspect.StreamQueryParams) (*introspectpb.StreamList, error) {
	swarm.conns.RLock()
	defer swarm.conns.RUnlock()

	containsId := func(ids []introspect.StreamID, id string) bool {
		for _, i := range ids {
			if string(i) == id {
				return true
			}
		}
		return false
	}

	sl := &introspectpb.StreamList{}

	appendStream := func(s *Stream) error {
		if query.Output == introspect.QueryOutputFull {
			is, err := swarm.introspectStream(s)
			if err != nil {
				return errors.Wrap(err, "failed to convert stream to introspect.Stream")
			}
			sl.Streams = append(sl.Streams, is)
		} else {
			sl.StreamIds = append(sl.StreamIds, []byte(s.id))
		}
		return nil
	}

	// iterate over all connections the swarm has & resolve the streams in the connections required by the query
	for _, conns := range swarm.conns.m {
		for _, c := range conns {
			// range over all streams in the connection
			c.streams.Lock()
			defer c.streams.Unlock()

			for s, _ := range c.streams.m {
				if query.Include != nil {
					if containsId(query.Include, s.id) {
						if err := appendStream(s); err != nil {
							return nil, err
						}
					}
				} else {
					if err := appendStream(s); err != nil {
						return nil, err
					}
				}
			}

		}
	}
	return sl, nil
}

func (swarm *Swarm) introspectStream(s *Stream) (*introspectpb.Stream, error) {
	s.state.Lock()
	defer s.state.Unlock()

	si := &streamIntrospector{swarm, s}
	is := &introspectpb.Stream{}
	is.Id = si.id()
	is.Protocol = si.protocol()
	is.Role = si.role()
	is.Status = si.status()
	is.Conn = si.conn()
	is.Traffic = si.traffic()
	is.Timeline = si.timeline()
	is.LatencyNs = si.latency()
	is.UserProvidedTags = si.userProvidedTags()
	return is, nil
}
