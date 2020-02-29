package QTransport
// This file contains the whole Transport to QTransport abstraction layer.

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

// 2^31
var defaultNonProxyQuality uint32 = 2147483648
// 2^31+2^30
var defaultProxyQuality uint32 = 3221225472

var _ transport.QTransport = TransportUpgrader{}

// Used to upgrade `transport.Transport` to `transport.QTransport`.
// Only use it with `BaseTransport` castable to `transport.Transport`.
type TransportUpgrader struct {
	transport.BaseTransport
}

func (t TransportUpgrader) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.QCapableConn, error) {
	conn, err := t.BaseTransport.(transport.Transport).Dial(ctx, raddr, p)
	if err != nil {
		return nil, err
	}
	return upgradedCapableConn{
		listenedUpgradedCapableConn{BaseCapableConn: conn, t: t},
	}, nil
}

func (t TransportUpgrader) Listen(laddr ma.Multiaddr) (transport.QListener, error) {
	l, err := t.BaseTransport.(transport.Transport).Listen(laddr)
	if err != nil {
		return nil, err
	}
	return upgradedListener{BaseListener: l, t: t}, nil
}

func (t TransportUpgrader) Score(raddr ma.Multiaddr, _ peer.ID) (transport.Score, error) {
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
	listenedUpgradedCapableConn
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

type listenedUpgradedCapableConn struct {
	transport.BaseCapableConn
	t transport.QTransport
}

func (c listenedUpgradedCapableConn) Transport() transport.QTransport {
	return c.t
}

type upgradedListener struct {
	transport.BaseListener
	t transport.QTransport
}

func (l upgradedListener) Accept() (transport.ListenedQCapableConn, error) {
	c, err := l.BaseListener.(transport.Listener).Accept()
	if err != nil {
		return nil, err
	}
	return listenedUpgradedCapableConn{BaseCapableConn: c, t: l.t}, nil
}
