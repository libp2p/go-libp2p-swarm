package swarm_test

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p-core/introspection"
	introspection_pb "github.com/libp2p/go-libp2p-core/introspection/pb"
	"github.com/libp2p/go-libp2p-core/protocol"
	swarm "github.com/libp2p/go-libp2p-swarm"

	"github.com/stretchr/testify/require"
)

func TestConnsAndStreamIntrospect(t *testing.T) {
	ctx := context.Background()
	swarms := makeSwarms(ctx, t, 2)
	connectSwarms(t, ctx, []*swarm.Swarm{swarms[0], swarms[1]})

	// ----- Swarm 1 opens TWO streams to Swarm 2
	pid1 := protocol.ID("1")
	pid2 := protocol.ID("2")
	s1, err := swarms[0].NewStream(ctx, swarms[1].LocalPeer())
	s1.SetProtocol(pid1)
	require.NoError(t, err)
	s2, err := swarms[0].NewStream(ctx, swarms[1].LocalPeer())
	require.NoError(t, err)
	s2.SetProtocol(pid2)

	// send 4 bytes on stream 1 & 5 bytes on stream 2
	msg1 := "abcd"
	msg2 := "12345"
	_, err = s1.Write([]byte(msg1))
	require.NoError(t, err)
	_, err = s2.Write([]byte(msg2))
	require.NoError(t, err)

	// wait for the metrics to kick in
	for {
		cis, err := swarms[0].IntrospectConnections(introspection.ConnectionQueryParams{Output: introspection.QueryOutputFull})
		require.NoError(t, err)

		if cis[0].Traffic.TrafficOut.CumBytes != 0 {
			break
		}
	}

	// ----- Introspect Swarm 1
	cis, err := swarms[0].IntrospectConnections(introspection.ConnectionQueryParams{Output: introspection.QueryOutputFull})
	require.NoError(t, err)

	// connection checks
	require.Len(t, cis, 1)
	require.Len(t, cis[0].Streams.StreamIds, 2)
	require.NotEmpty(t, cis[0].Id)
	require.Equal(t, swarms[1].LocalPeer().String(), cis[0].PeerId)
	require.Equal(t, introspection_pb.Status_ACTIVE, cis[0].Status)
	require.Equal(t, introspection_pb.Role_INITIATOR, cis[0].Role)
	require.Equal(t, swarms[0].Conns()[0].LocalMultiaddr().String(), cis[0].Endpoints.SrcMultiaddr)
	require.Equal(t, swarms[0].Conns()[0].RemoteMultiaddr().String(), cis[0].Endpoints.DstMultiaddr)
	require.True(t, int(cis[0].Traffic.TrafficOut.CumBytes) == len(msg1)+len(msg2))

	// verify we get connectionIds correctly
	cids, err := swarms[0].IntrospectConnections(introspection.ConnectionQueryParams{Output: introspection.QueryOutputList})
	require.NoError(t, err)
	require.Len(t, cids, 1)
	require.NotEmpty(t, cids[0].Id)
	require.Empty(t, cids[0].PeerId)

	// verify we get the same result if we pass in the connection Ids
	cs, err := swarms[0].IntrospectConnections(introspection.ConnectionQueryParams{introspection.QueryOutputFull,
		[]introspection.ConnectionID{introspection.ConnectionID(cis[0].Id)}})
	require.NoError(t, err)
	require.Len(t, cs, 1)
	require.Equal(t, cis[0].PeerId, cs[0].PeerId)
	require.Equal(t, cis[0].Id, cs[0].Id)

	// fetch streams by reading Ids from connection
	var sids []introspection.StreamID
	for _, s := range cis[0].Streams.StreamIds {
		sids = append(sids, introspection.StreamID(s))
	}

	// Now, introspect Streams
	sl, err := swarms[0].IntrospectStreams(introspection.StreamQueryParams{introspection.QueryOutputFull, sids})
	require.Len(t, sl.Streams, 2)
	require.NoError(t, err)

	// map stream to protocols
	protocolToStream := make(map[string]*introspection_pb.Stream)
	for _, s := range sl.Streams {
		protocolToStream[s.Protocol] = s
	}

	// introspect stream 1
	stream1 := protocolToStream["1"]
	require.NotEmpty(t, stream1)
	require.Equal(t, "1", stream1.Protocol)
	require.Equal(t, introspection_pb.Role_INITIATOR, stream1.Role)
	require.Equal(t, introspection_pb.Status_ACTIVE, stream1.Status)
	require.NotEmpty(t, stream1.Id)
	require.NotNil(t, stream1.Traffic)
	require.NotNil(t, stream1.Traffic.TrafficIn)
	require.NotNil(t, stream1.Traffic.TrafficOut)

	// require.True(t, len(msg1) == int(stream1.Traffic.TrafficOut.CumBytes))
	// require.True(t, 0 == int(stream1.Traffic.TrafficIn.CumBytes))

	// introspect stream 2
	stream2 := protocolToStream["2"]
	require.NotEmpty(t, stream2)
	require.Equal(t, "2", stream2.Protocol)
	require.Equal(t, introspection_pb.Role_INITIATOR, stream2.Role)
	require.Equal(t, introspection_pb.Status_ACTIVE, stream2.Status)
	require.NotEmpty(t, stream2.Id)
	require.NotEqual(t, stream2.Id, stream1.Id)
	// require.True(t, len(msg2) == int(stream2.Traffic.TrafficOut.CumBytes))
	// require.True(t, 0 == int(stream2.Traffic.TrafficIn.CumBytes))

	// Assert query ONLY for streaIds
	streamList, err := swarms[0].IntrospectStreams(introspection.StreamQueryParams{Output: introspection.QueryOutputList})
	require.NoError(t, err)
	require.Len(t, streamList.Streams, 0)
	require.Len(t, streamList.StreamIds, 2)

	// reset stream 1 & verify
	require.NoError(t, s1.Reset())
	cis, err = swarms[0].IntrospectConnections(introspection.ConnectionQueryParams{Output: introspection.QueryOutputFull})
	require.NoError(t, err)
	require.Len(t, cis[0].Streams.StreamIds, 1)

	// introspect traffic
	tr, err := swarms[0].IntrospectTraffic()
	require.NoError(t, err)
	require.True(t, tr.TrafficOut.CumBytes == uint64(len(msg1)+len(msg2)))
	require.True(t, tr.TrafficIn.CumBytes == 0)
}
