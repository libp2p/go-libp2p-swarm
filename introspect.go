package swarm

import (
	"fmt"
	"math"

	"github.com/libp2p/go-libp2p-core/introspection"
	introspect_pb "github.com/libp2p/go-libp2p-core/introspection/pb"
	introspection_pb "github.com/libp2p/go-libp2p-core/introspection/pb"
	"github.com/libp2p/go-libp2p-core/network"

	"github.com/gogo/protobuf/types"
)

// IntrospectTraffic introspects and returns total traffic stats for this swarm.
func (s *Swarm) IntrospectTraffic() (*introspection_pb.Traffic, error) {
	if s.bwc == nil {
		return nil, nil
	}

	metrics := s.bwc.GetBandwidthTotals()
	t := &introspection_pb.Traffic{
		TrafficIn: &introspection_pb.DataGauge{
			CumBytes: uint64(metrics.TotalIn),
			InstBw:   uint64(metrics.RateIn),
		},
		TrafficOut: &introspection_pb.DataGauge{
			CumBytes: uint64(metrics.TotalOut),
			InstBw:   uint64(metrics.RateOut),
		},
	}

	return t, nil
}

// IntrospectConnections introspects & returns the swarm connections
func (s *Swarm) IntrospectConnections(q introspection.ConnectionQueryParams) ([]*introspection_pb.Connection, error) {
	var conns []network.Conn

	switch l := len(q.Include); l {
	case 0:
		conns = s.Conns()

	default:
		conns = make([]network.Conn, 0, l)
		filter := make(map[string]struct{}, l)
		for _, id := range q.Include {
			filter[string(id)] = struct{}{}
		}

		for _, c := range s.Conns() {
			if _, ok := filter[c.(*Conn).ID()]; !ok {
				continue
			}
			conns = append(conns, c)
		}
	}

	introspected := make([]*introspection_pb.Connection, 0, len(conns))

	switch q.Output {
	case introspection.QueryOutputFull:
		for _, c := range conns {
			ic, err := c.(*Conn).Introspect(s, q)
			if err != nil {
				return nil, fmt.Errorf("failed to introspect conneciton, err=%s", err)
			}
			introspected = append(introspected, ic)
		}

	case introspection.QueryOutputList:
		for _, c := range conns {
			introspected = append(introspected, &introspection_pb.Connection{Id: []byte(c.(*Conn).ID())})
		}

	default:
		return nil, fmt.Errorf("unexpected query type: %v", q.Output)
	}

	return introspected, nil
}

// IntrospectStreams processes a streams introspection query.
func (s *Swarm) IntrospectStreams(q introspection.StreamQueryParams) (*introspection_pb.StreamList, error) {
	var streams []network.Stream

	switch l := len(q.Include); l {
	case 0:
		for _, c := range s.Conns() {
			for _, s := range c.GetStreams() {
				streams = append(streams, s)
			}
		}

	default:
		streams = make([]network.Stream, 0, l)
		filter := make(map[string]struct{}, l)
		for _, id := range q.Include {
			filter[string(id)] = struct{}{}
		}

		for _, c := range s.Conns() {
			for _, s := range c.GetStreams() {
				if _, ok := filter[s.(*Stream).ID()]; !ok {
					continue
				}
				streams = append(streams, s)
			}
		}
	}

	switch q.Output {
	case introspection.QueryOutputFull:
		introspected := make([]*introspection_pb.Stream, 0, len(streams))
		for _, st := range streams {
			is, err := st.(*Stream).Introspect(s, q)
			if err != nil {
				return nil, fmt.Errorf("failed to introspect stream, err=%s", err)
			}
			introspected = append(introspected, is)
		}
		return &introspection_pb.StreamList{Streams: introspected}, nil

	case introspection.QueryOutputList:
		introspected := make([][]byte, 0, len(streams))
		for _, st := range streams {
			introspected = append(introspected, []byte(st.(*Stream).ID()))
		}
		return &introspection_pb.StreamList{StreamIds: introspected}, nil
	}

	return nil, fmt.Errorf("unexpected query type: %v", q.Output)
}

func (c *Conn) Introspect(s *Swarm, q introspection.ConnectionQueryParams) (*introspect_pb.Connection, error) {
	stat := c.Stat()

	openTs, err := types.TimestampProto(stat.Opened)
	if err != nil {
		return nil, fmt.Errorf("failed to convert open time to proto, err=%s", err)
	}

	res := &introspection_pb.Connection{
		Id:     []byte(c.ID()),
		Status: introspection_pb.Status_ACTIVE,
		PeerId: c.RemotePeer().Pretty(),
		Endpoints: &introspection_pb.EndpointPair{
			SrcMultiaddr: c.LocalMultiaddr().String(),
			DstMultiaddr: c.RemoteMultiaddr().String(),
		},
		Role: translateRole(stat),

		Timeline: &introspection_pb.Connection_Timeline{
			OpenTs:     openTs,
			UpgradedTs: openTs,
			// TODO ClosedTs, UpgradedTs.
		},
	}

	// TODO this is a per-peer, not a per-conn measurement. In the future, when
	// we have multiple connections per peer, this will produce inaccurate
	// numbers.
	// Also, we do not record stream-level stats.
	if s.bwc != nil {
		bw := s.bwc.GetBandwidthForPeer(c.RemotePeer())
		res.Traffic = &introspect_pb.Traffic{
			// TODO we don't have packet I/O stats.
			TrafficIn: &introspect_pb.DataGauge{
				CumBytes: uint64(bw.TotalIn),
				InstBw:   uint64(math.Round(bw.RateIn)),
			},
			TrafficOut: &introspect_pb.DataGauge{
				CumBytes: uint64(bw.TotalOut),
				InstBw:   uint64(math.Round(bw.RateOut)),
			},
		}
	}

	// TODO I don't think we pin the multiplexer and the secure channel we've
	// negotiated anywhere.
	res.Attribs = &introspect_pb.Connection_Attributes{}

	// TODO can we get the transport ID from the multiaddr?
	res.TransportId = nil

	// TODO there's the ping protocol, but that's higher than this layer.
	// How do we source this? We may need some kind of latency manager.
	res.LatencyNs = 0

	c.streams.Lock()
	sids := make([][]byte, 0, len(c.streams.m))
	for s := range c.streams.m {
		sids = append(sids, []byte(s.ID()))
	}
	c.streams.Unlock()

	res.Streams = &introspection_pb.StreamList{StreamIds: sids}

	return res, nil
}

func (s *Stream) Introspect(sw *Swarm, q introspection.StreamQueryParams) (*introspect_pb.Stream, error) {
	stat := s.Stat()
	openTs, err := types.TimestampProto(stat.Opened)
	if err != nil {
		return nil, fmt.Errorf("failed to convert open time to proto, err=%s", err)
	}

	res := &introspection_pb.Stream{
		Id:     []byte(s.ID()),
		Status: introspect_pb.Status_ACTIVE,
		Conn: &introspect_pb.Stream_ConnectionRef{
			Connection: &introspection_pb.Stream_ConnectionRef_ConnId{
				ConnId: []byte(s.conn.ID()),
			},
		},
		Protocol: string(s.Protocol()),
		Role:     translateRole(stat),
		Timeline: &introspect_pb.Stream_Timeline{
			OpenTs: openTs,
			// TODO CloseTs.
		},
		// TODO Traffic: we are not tracking per-stream traffic stats at the
		Traffic: &introspection_pb.Traffic{&introspection_pb.DataGauge{}, &introspection_pb.DataGauge{}},
		// moment.
	}

	return res, nil
}

func translateRole(stat network.Stat) introspect_pb.Role {
	switch stat.Direction {
	case network.DirInbound:
		return introspect_pb.Role_RESPONDER
	case network.DirOutbound:
		return introspect_pb.Role_INITIATOR
	default:
		return 99 // TODO placeholder value
	}
}
