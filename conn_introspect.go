package swarm

import (
	"github.com/libp2p/go-libp2p-core/introspect"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/pkg/errors"
)

type connIntrospector struct {
	c     *Conn
	s     *Swarm
	query *introspect.ConnectionQueryInput
}

func (ci *connIntrospector) id() string {
	return ci.c.id
}

func (ci *connIntrospector) remotePeerId() string {
	return ci.c.RemotePeer().String()
}

func (ci *connIntrospector) endPoints() *introspect.EndpointPair {
	return &introspect.EndpointPair{SrcMultiaddr: ci.c.LocalMultiaddr().String(), DstMultiaddr: ci.c.RemoteMultiaddr().String()}
}

func (ci *connIntrospector) role() introspect.Role {
	if ci.c.stat.Direction == network.DirInbound {
		return introspect.Role_RESPONDER
	} else {
		return introspect.Role_INITIATOR
	}
}

// TODO Number of packets & instantaneous bandwidth ?
// Should we have a separate "flow-metre" for a connection to prepare for a message oriented world ?
func (ci *connIntrospector) traffic() *introspect.Traffic {
	if ci.s.bwc != nil {
		t := &introspect.Traffic{}
		t.TrafficIn = &introspect.DataGauge{}
		t.TrafficOut = &introspect.DataGauge{}
		for s, _ := range ci.c.streams.m {
			streamMetrics := ci.s.bwc.GetBandwidthForProtocol(s.Protocol())
			t.TrafficIn.CumBytes = t.TrafficIn.CumBytes + uint64(streamMetrics.TotalIn)
			t.TrafficIn.InstBw = t.TrafficIn.InstBw + uint64(streamMetrics.RateIn)

			t.TrafficOut.CumBytes = t.TrafficOut.CumBytes + uint64(streamMetrics.TotalOut)
			t.TrafficOut.InstBw = t.TrafficOut.CumBytes + uint64(streamMetrics.RateOut)
		}
		return t
	}

	return nil
}

func (ci *connIntrospector) streams() (*introspect.StreamList, error) {
	sl := &introspect.StreamList{}
	for s, _ := range ci.c.streams.m {
		switch ci.query.StreamOutputType {
		case introspect.QueryOutputTypeFull:
			isl, err := ci.s.introspectStream(s, ci.c)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert swarm stream to introspect.Stream")
			}
			sl.Streams = append(sl.Streams, isl)
		case introspect.QueryOutputTypeIds:
			sl.StreamIds = append(sl.StreamIds, []byte(s.id))
		}
	}
	return sl, nil
}

// TODO Where do we get this information from ?
func (ci *connIntrospector) attribs() *introspect.Connection_Attributes {
	return nil
}

// TODO Where are the hook points and where do we persist closed time ?
func (ci *connIntrospector) timeline() *introspect.Connection_Timeline {
	return nil
}

// TODO How do we track othe states ?
func (ci *connIntrospector) status() introspect.Status {
	return introspect.Status_ACTIVE
}

//	TODO What's this & how to fetch it ?
func (ci *connIntrospector) transportId() []byte {
	return nil
}

// TODO How ?
func (ci *connIntrospector) latencyNs() uint64 {
	return 0
}

// TODO What's this ?
func (ci *connIntrospector) userProvidedTags() []string {
	return nil
}

// TODO What's this & how to fetch it ?
func (ci *connIntrospector) relayedOver() *introspect.Connection_Conn {
	return nil
}
