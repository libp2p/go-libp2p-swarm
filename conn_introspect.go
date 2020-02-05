package swarm

import (
	introspectpb "github.com/libp2p/go-libp2p-core/introspect/pb"
	"github.com/libp2p/go-libp2p-core/network"
)

type connIntrospector struct {
	c *Conn
	s *Swarm
}

func (ci *connIntrospector) id() string {
	return ci.c.id
}

func (ci *connIntrospector) remotePeerId() string {
	return ci.c.RemotePeer().String()
}

func (ci *connIntrospector) endPoints() *introspectpb.EndpointPair {
	return &introspectpb.EndpointPair{SrcMultiaddr: ci.c.LocalMultiaddr().String(), DstMultiaddr: ci.c.RemoteMultiaddr().String()}
}

func (ci *connIntrospector) role() introspectpb.Role {
	if ci.c.stat.Direction == network.DirInbound {
		return introspectpb.Role_RESPONDER
	} else {
		return introspectpb.Role_INITIATOR
	}
}

// TODO Number of packets & instantaneous bandwidth ?
// Should we have a separate "flow-metre" for a connection to prepare for a message oriented world ?
func (ci *connIntrospector) traffic() *introspectpb.Traffic {
	if ci.s.bwc != nil {
		t := &introspectpb.Traffic{}
		t.TrafficIn = &introspectpb.DataGauge{}
		t.TrafficOut = &introspectpb.DataGauge{}
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

func (ci *connIntrospector) streams() (*introspectpb.StreamList, error) {
	sl := &introspectpb.StreamList{}
	for s, _ := range ci.c.streams.m {
		sl.StreamIds = append(sl.StreamIds, []byte(s.id))
	}
	return sl, nil
}

// TODO Where do we get this information from ?
func (ci *connIntrospector) attribs() *introspectpb.Connection_Attributes {
	return nil
}

// TODO Where are the hook points and where do we persist closed time ?
func (ci *connIntrospector) timeline() *introspectpb.Connection_Timeline {
	return nil
}

// TODO How do we track othe states ?
func (ci *connIntrospector) status() introspectpb.Status {
	return introspectpb.Status_ACTIVE
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
func (ci *connIntrospector) relayedOver() *introspectpb.Connection_ConnId {
	return nil
}
