package swarm

import (
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/libp2p/go-libp2p-core/introspect"
	"github.com/libp2p/go-libp2p-core/network"
)

type streamIntrospector struct {
	s *Stream
	c *Conn
}

func (si *streamIntrospector) id() string {
	return si.s.id
}

func (si *streamIntrospector) protocol() string {
	return string(si.s.Protocol())
}

func (si *streamIntrospector) status() introspect.Status {
	switch si.s.state.v {
	case streamOpen:
		return introspect.Status_ACTIVE
	case streamReset:
		return introspect.Status_ERROR
	case streamCloseBoth:
		return introspect.Status_CLOSED
	default:
		return introspect.Status_ACTIVE
	}
}

func (si *streamIntrospector) role() introspect.Role {
	if si.s.stat.Direction == network.DirInbound {
		return introspect.Role_RESPONDER
	} else {
		return introspect.Role_INITIATOR
	}
}

func (si *streamIntrospector) conn() *introspect.Stream_ConnectionRef {
	return &introspect.Stream_ConnectionRef{Connection: &introspect.Stream_ConnectionRef_ConnId{si.c.id}}
}

// TODO Number packets ? Is the RateIn/Out a good approximation here ?
func (si *streamIntrospector) traffic() *introspect.Traffic {
	if si.c.swarm.bwc != nil {
		streamMetrics := si.s.conn.swarm.bwc.GetBandwidthForProtocol(si.s.Protocol())
		t := &introspect.Traffic{}
		t.TrafficIn = &introspect.DataGauge{CumBytes: uint64(streamMetrics.TotalIn), InstBw: uint64(streamMetrics.RateIn)}
		t.TrafficOut = &introspect.DataGauge{CumBytes: uint64(streamMetrics.TotalOut), InstBw: uint64(streamMetrics.RateOut)}
		return t
	}

	return nil
}

// TODO What about closed time ?
func (si *streamIntrospector) timeline() *introspect.Stream_Timeline {
	return &introspect.Stream_Timeline{OpenTs: &timestamp.Timestamp{Seconds: si.s.openTime.Unix()}}
}

// TODO How ?
func (si *streamIntrospector) latency() uint64 {
	return 0
}

// TODO What's this ?
func (si *streamIntrospector) userProvidedTags() []string {
	return nil
}
