package swarm

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/transport"

	ma "github.com/multiformats/go-multiaddr"
)

// TransportForDialing retrieves the appropriate transport for dialing the given
// multiaddr.
func (s *Swarm) TransportForDialing(a ma.Multiaddr) transport.Transport {
	protocols := a.Protocols()
	if len(protocols) == 0 {
		return nil
	}

	s.transports.RLock()
	defer s.transports.RUnlock()
	if len(s.transports.m) == 0 {
		// make sure we're not just shutting down.
		if s.transports.m != nil {
			log.Error("you have no transports configured")
		}
		return nil
	}

	for _, p := range protocols {
		transport, ok := s.transports.m[transportsToMapKey([]ma.Protocol{p})]
		if !ok {
			continue
		}
		if transport.Proxy() {
			return transport
		}
	}

	if addrContainsSecurityProtocol(a) {
		return s.transports.m[transportsToMapKey(protocols[len(protocols)-2:])]
	}
	return s.transports.m[transportsToMapKey(protocols[len(protocols)-1:])]
}

// TransportForListening retrieves the appropriate transport for listening on
// the given multiaddr.
func (s *Swarm) TransportForListening(a ma.Multiaddr) transport.Transport {
	protocols := a.Protocols()
	if len(protocols) == 0 {
		return nil
	}

	s.transports.RLock()
	defer s.transports.RUnlock()
	if len(s.transports.m) == 0 {
		// make sure we're not just shutting down.
		if s.transports.m != nil {
			log.Error("you have no transports configured")
		}
		return nil
	}

	for _, p := range protocols {
		transport, ok := s.transports.m[transportsToMapKey([]ma.Protocol{p})]
		if !ok {
			continue
		}
		if transport.Proxy() {
			return transport
		}
	}

	if addrContainsSecurityProtocol(a) {
		return s.transports.m[transportsToMapKey(protocols[len(protocols)-2:])]
	}
	return s.transports.m[transportsToMapKey(protocols[len(protocols)-1:])]
}

// AddTransport adds a Transport to this swarm.
func (s *Swarm) AddTransport(t transport.Transport) error {
	protocols := t.Protocols()
	if len(protocols) == 0 {
		return fmt.Errorf("useless transport handles no protocols: %T", t)
	}
	// Examples:
	// * for TCP (doing handshake protocol negotiation): tcp
	// * for TCP / TLS (handling multiaddrs containing tls): tcp/tls
	transportKey := transportsToMapKey(protocols)

	s.transports.Lock()
	defer s.transports.Unlock()
	if s.transports.m == nil {
		return ErrSwarmClosed
	}
	if _, ok := s.transports.m[transportKey]; ok {
		// TODO: improve error message
		return fmt.Errorf("transport already registered for protocol: %s", transportKey)
	}

	s.transports.m[transportKey] = t
	return nil
}

func transportsToMapKey(protocols []ma.Protocol) string {
	var key string
	for i, p := range protocols {
		if i > 0 {
			key += "/"
		}
		key += p.Name
	}
	return key
}

func addrContainsSecurityProtocol(addr ma.Multiaddr) bool {
	var contains bool
	ma.ForEach(addr, func(c ma.Component) bool {
		if code := c.Protocol().Code; code == ma.P_TLS || code == ma.P_NOISE || code == ma.P_PLAINTEXTV2 {
			contains = true
		}
		return true
	})
	return contains
}
