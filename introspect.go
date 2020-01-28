package swarm

import (
	"github.com/libp2p/go-libp2p-core/introspect"
	"github.com/pkg/errors"
)

// IntrospectTraffic introspects & returns the overall traffic for this peer
func (s *Swarm) IntrospectTraffic() (*introspect.Traffic, error) {
	if s.bwc != nil {
		t := &introspect.Traffic{}
		metrics := s.bwc.GetBandwidthTotals()

		t.TrafficIn = &introspect.DataGauge{CumBytes: uint64(metrics.TotalIn), InstBw: uint64(metrics.RateIn)}
		t.TrafficOut = &introspect.DataGauge{CumBytes: uint64(metrics.TotalOut), InstBw: uint64(metrics.RateOut)}

		return t, nil
	}

	return nil, nil
}

// IntrospectConns introspects & returns the swarm connections
func (swarm *Swarm) IntrospectConns(query introspect.ConnectionQueryInput) ([]*introspect.Connection, error) {
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

	// iterate over all connections the swarm has & resolve the ones required by the query
	for _, conns := range swarm.conns.m {
		for _, c := range conns {
			switch query.Type {
			case introspect.ConnListQueryTypeForIds:
				if containsId(query.ConnIDs, c.id) {
					ic, err := swarm.introspectConnection(&query, c)
					if err != nil {
						return nil, errors.Wrap(err, "failed to convert connection to introspect.Connection")
					}
					iconns = append(iconns, ic)
				}
			case introspect.ConnListQueryTypeAll:
				ic, err := swarm.introspectConnection(&query, c)
				if err != nil {
					return nil, errors.Wrap(err, "failed to convert connection to introspect.Connection")
				}
				iconns = append(iconns, ic)
			}
		}
	}
	return iconns, nil
}

func (swarm *Swarm) introspectConnection(query *introspect.ConnectionQueryInput, c *Conn) (*introspect.Connection, error) {
	c.streams.Lock()
	defer c.streams.Unlock()

	ci := &connIntrospector{c, swarm, query}
	ic := &introspect.Connection{}

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

func (swarm *Swarm) introspectStream(s *Stream, c *Conn) (*introspect.Stream, error) {
	s.state.Lock()
	defer s.state.Unlock()

	si := &streamIntrospector{s, c}
	is := &introspect.Stream{}
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
