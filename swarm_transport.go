package swarm

import (
	"context"
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

// 2^31
const defaultNonProxyQuality = 2147483648

// 2^31+2^30
const defaultProxyQuality = 3221225472

// TransportForDialing retrieves the appropriate transport for dialing the given
// multiaddr.
func (s *Swarm) TransportForDialing(a ma.Multiaddr) transport.QTransport {
	protocols := a.Protocols()
	if len(protocols) == 0 {
		return nil
	}

	s.transports.RLock()
	defer s.transports.RUnlock()
	if len(s.transports.m) == 0 {
		log.Error("you have no transports configured")
		return nil
	}

	for _, p := range protocols {
		transport, ok := s.transports.m[p.Code]
		if !ok {
			continue
		}
		if transport.Proxy() {
			return transport
		}
	}

	return s.transports.m[protocols[len(protocols)-1].Code]
}

// TransportForListening retrieves the appropriate transport for listening on
// the given multiaddr.
func (s *Swarm) TransportForListening(a ma.Multiaddr) transport.QTransport {
	protocols := a.Protocols()
	if len(protocols) == 0 {
		return nil
	}

	s.transports.RLock()
	defer s.transports.RUnlock()
	if len(s.transports.m) == 0 {
		log.Error("you have no transports configured")
		return nil
	}

	selected := s.transports.m[protocols[len(protocols)-1].Code]
	for _, p := range protocols {
		transport, ok := s.transports.m[p.Code]
		if !ok {
			continue
		}
		if transport.Proxy() {
			selected = transport
		}
	}
	return selected
}

var errTransportUncastable = fmt.Errorf("BaseTransport must be `transport.QTransport` or `transport.Transport`.")

// AddTransport adds a transport to this swarm.
//
// Satisfies the Network interface from go-libp2p-transport.
func (s *Swarm) AddTransport(ot transport.BaseTransport) error {
	var t transport.QTransport
	switch rt := ot.(type) {
	case transport.QTransport:
		t = rt
	case transport.Transport:
		t = transportUpgrader{BaseTransport: rt}
	default:
		return errTransportUncastable
	}
	protocols := t.Protocols()

	if len(protocols) == 0 {
		return fmt.Errorf("useless transport handles no protocols: %T", t)
	}

	s.transports.Lock()
	defer s.transports.Unlock()
	var registered []string
	for _, p := range protocols {
		if _, ok := s.transports.m[p]; ok {
			proto := ma.ProtocolWithCode(p)
			name := proto.Name
			if name == "" {
				name = fmt.Sprintf("unknown (%d)", p)
			}
			registered = append(registered, name)
		}
	}
	if len(registered) > 0 {
		return fmt.Errorf(
			"transports already registered for protocol(s): %s",
			strings.Join(registered, ", "),
		)
	}

	for _, p := range protocols {
		s.transports.m[p] = t
	}
	return nil
}

// Used to upgrade `transport.Transport` to `transport.QTransport`.
// Only use it with `BaseTransport` castable to `transport.Transport`.
type transportUpgrader struct {
	transport.BaseTransport
}

func (t transportUpgrader) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.QCapableConn, error) {
	conn, err := t.BaseTransport.(transport.Transport).Dial(ctx, raddr, p)
	if err != nil {
		return nil, err
	}
	return upgradedCapableConn{CapableConn: conn}, nil
}

func (t transportUpgrader) Score(raddr ma.Multiaddr, _ peer.ID) (transport.Score, error) {
	if !t.CanDial(raddr) {
		return transport.Score{}, fmt.Errorf("\"%s\" does not match %s.", raddr, t.BaseTransport)
	}
	if t.Proxy() {
		if manet.IsIPLoopback(raddr) {
			return transport.Score{
				Quality:   defaultProxyQuality >> 16,
				IsQuality: true,
				Fd:        1,
			}, nil
		}
		if manet.IsPrivateAddr(raddr) {
			return transport.Score{
				Quality:   defaultProxyQuality >> 8,
				IsQuality: true,
				Fd:        1,
			}, nil
		}
		return transport.Score{
			Quality:   defaultProxyQuality,
			IsQuality: true,
			Fd:        1,
		}, nil
	}
	if manet.IsIPLoopback(raddr) {
		return transport.Score{
			Quality:   defaultNonProxyQuality >> 16,
			IsQuality: true,
			Fd:        1,
		}, nil
	}
	if manet.IsPrivateAddr(raddr) {
		return transport.Score{
			Quality:   defaultNonProxyQuality >> 8,
			IsQuality: true,
			Fd:        1,
		}, nil
	}
	return transport.Score{
		Quality:   defaultNonProxyQuality,
		IsQuality: true,
		Fd:        1,
	}, nil
}

// Used to upgrade `transport.CapableConn` to `transport.QCapableConn`.
type upgradedCapableConn struct {
	transport.CapableConn
}

func (c upgradedCapableConn) Quality() uint32 {
	raddr := c.RemoteMultiaddr()
	if c.Transport().Proxy() {
		if manet.IsIPLoopback(raddr) {
			return defaultProxyQuality >> 16
		}
		if manet.IsPrivateAddr(raddr) {
			return defaultProxyQuality >> 8
		}
		return defaultProxyQuality
	}
	if manet.IsIPLoopback(raddr) {
		return defaultNonProxyQuality >> 16
	}
	if manet.IsPrivateAddr(raddr) {
		return defaultNonProxyQuality >> 8
	}
	return defaultNonProxyQuality
}
