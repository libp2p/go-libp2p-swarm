package swarm_test

import (
	"context"
	"github.com/libp2p/go-libp2p-core/introspect"
	"github.com/libp2p/go-libp2p-core/protocol"
	swarm "github.com/libp2p/go-libp2p-swarm"
	"github.com/stretchr/testify/require"
	"testing"
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
	s1.Write([]byte(msg1))
	s2.Write([]byte(msg2))
	// wait for the metres to kick in
	for {
		cis, err := swarms[0].IntrospectConns(introspect.ConnectionQueryInput{Type: introspect.ConnListQueryTypeAll,
			StreamOutputType: introspect.QueryOutputTypeFull})
		require.NoError(t, err)
		if cis[0].Traffic.TrafficOut.CumBytes != 0 {
			break
		}
	}

	// ----- Introspect Swarm 1
	cis, err := swarms[0].IntrospectConns(introspect.ConnectionQueryInput{Type: introspect.ConnListQueryTypeAll,
		StreamOutputType: introspect.QueryOutputTypeFull})

	// connection checks
	require.Len(t, cis, 1)
	require.Len(t, cis[0].Streams.Streams, 2)
	require.NotEmpty(t, cis[0].Id)
	require.Equal(t, swarms[1].LocalPeer().String(), cis[0].PeerId)
	require.Equal(t, introspect.Status_ACTIVE, cis[0].Status)
	require.Equal(t, introspect.Role_INITIATOR, cis[0].Role)
	require.Equal(t, swarms[0].Conns()[0].LocalMultiaddr().String(), cis[0].Endpoints.SrcMultiaddr)
	require.Equal(t, swarms[0].Conns()[0].RemoteMultiaddr().String(), cis[0].Endpoints.DstMultiaddr)
	require.True(t, int(cis[0].Traffic.TrafficOut.CumBytes) == len(msg1)+len(msg2))

	// map stream to protocols
	protocolToStream := make(map[string]*introspect.Stream)
	for _, s := range cis[0].Streams.Streams {
		protocolToStream[s.Protocol] = s
	}

	// introspect stream 1
	stream1 := protocolToStream["1"]
	require.NotEmpty(t, stream1)
	require.Equal(t, "1", stream1.Protocol)
	require.Equal(t, introspect.Role_INITIATOR, stream1.Role)
	require.Equal(t, introspect.Status_ACTIVE, stream1.Status)
	require.NotEmpty(t, stream1.Id)
	require.True(t, len(msg1) == int(stream1.Traffic.TrafficOut.CumBytes))
	require.True(t, 0 == int(stream1.Traffic.TrafficIn.CumBytes))

	// introspect stream 2
	stream2 := protocolToStream["2"]
	require.NotEmpty(t, stream2)
	require.Equal(t, "2", stream2.Protocol)
	require.Equal(t, introspect.Role_INITIATOR, stream2.Role)
	require.Equal(t, introspect.Status_ACTIVE, stream2.Status)
	require.NotEmpty(t, stream2.Id)
	require.NotEqual(t, stream2.Id, stream1.Id)
	require.True(t, len(msg2) == int(stream2.Traffic.TrafficOut.CumBytes))
	require.True(t, 0 == int(stream2.Traffic.TrafficIn.CumBytes))

	// reset stream 1 & verify
	require.NoError(t, s1.Reset())
	cis, err = swarms[0].IntrospectConns(introspect.ConnectionQueryInput{Type: introspect.ConnListQueryTypeAll,
		StreamOutputType: introspect.QueryOutputTypeFull})
	require.NoError(t, err)
	require.Len(t, cis[0].Streams.Streams, 1)
}
