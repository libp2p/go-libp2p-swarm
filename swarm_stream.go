package swarm

import (
	inet "github.com/libp2p/go-libp2p/p2p/net"
	protocol "github.com/libp2p/go-libp2p/p2p/protocol"

	ps "github.com/jbenet/go-peerstream"
)

// Stream is a wrapper around a ps.Stream that exposes a way to get
// our Conn and Swarm (instead of just the ps.Conn and ps.Swarm)
type Stream struct {
	stream   *ps.Stream
	protocol protocol.ID
}

// Stream returns the underlying peerstream.Stream
func (s *Stream) Stream() *ps.Stream {
	return s.stream
}

// Conn returns the Conn associated with this Stream, as an inet.Conn
func (s *Stream) Conn() inet.Conn {
	return s.SwarmConn()
}

// SwarmConn returns the Conn associated with this Stream, as a *Conn
func (s *Stream) SwarmConn() *Conn {
	return (*Conn)(s.stream.Conn())
}

// Read reads bytes from a stream.
func (s *Stream) Read(p []byte) (n int, err error) {
	return s.stream.Read(p)
}

// Write writes bytes to a stream, flushing for each call.
func (s *Stream) Write(p []byte) (n int, err error) {
	return s.stream.Write(p)
}

// Close closes the stream, indicating this side is finished
// with the stream.
func (s *Stream) Close() error {
	return s.stream.Close()
}

func (s *Stream) Protocol() protocol.ID {
	return s.protocol
}

func (s *Stream) SetProtocol(p protocol.ID) {
	s.protocol = p
}

func wrapStream(pss *ps.Stream) *Stream {
	return &Stream{
		stream: pss,
	}
}

func wrapStreams(st []*ps.Stream) []*Stream {
	out := make([]*Stream, len(st))
	for i, s := range st {
		out[i] = wrapStream(s)
	}
	return out
}
