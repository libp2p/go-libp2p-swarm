package swarm

import (
	introspectpb "github.com/libp2p/go-libp2p-core/introspect/pb"
	"github.com/libp2p/go-libp2p-core/network"
)

type streamIntrospector struct {
	swarm *Swarm
	s     *Stream
}

func (si *streamIntrospector) id() string {
	return si.s.id
}

func (si *streamIntrospector) protocol() string {
	return string(si.s.Protocol())
}

func (si *streamIntrospector) status() introspectpb.Status {
	switch si.s.state.v {
	case streamOpen:
		return introspectpb.Status_ACTIVE
	case streamReset:
		return introspectpb.Status_ERROR
	case streamCloseBoth:
		return introspectpb.Status_CLOSED
	default:
		return introspectpb.Status_ACTIVE
	}
}

func (si *streamIntrospector) role() introspectpb.Role {
	if si.s.stat.Direction == network.DirInbound {
		return introspectpb.Role_RESPONDER
	} else {
		return introspectpb.Role_INITIATOR
	}
}

func (si *streamIntrospector) conn() *introspectpb.Stream_ConnectionRef {
	return &introspectpb.Stream_ConnectionRef{Connection: &introspectpb.Stream_ConnectionRef_ConnId{si.s.conn.id}}
}

// TODO Number packets ? Is the RateIn/Out a good approximation here ?
func (si *streamIntrospector) traffic() *introspectpb.Traffic {
	if si.swarm.bwc != nil {
		streamMetrics := si.s.conn.swarm.bwc.GetBandwidthForProtocol(si.s.Protocol())
		t := &introspectpb.Traffic{}
		t.TrafficIn = &introspectpb.DataGauge{CumBytes: uint64(streamMetrics.TotalIn), InstBw: uint64(streamMetrics.RateIn)}
		t.TrafficOut = &introspectpb.DataGauge{CumBytes: uint64(streamMetrics.TotalOut), InstBw: uint64(streamMetrics.RateOut)}
		return t
	}

	return nil
}

// TODO What about closed time ?
func (si *streamIntrospector) timeline() *introspectpb.Stream_Timeline {
	return &introspectpb.Stream_Timeline{OpenTs: &si.s.openTime}
}

// TODO How ?
func (si *streamIntrospector) latency() uint64 {
	return 0
}

// TODO What's this ?
func (si *streamIntrospector) userProvidedTags() []string {
	return nil
}
