package testing

import (
	"testing"

	"github.com/stretchr/testify/require"

	csms "github.com/libp2p/go-conn-security-multistream"
	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/control"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/sec/insecure"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	quic "github.com/libp2p/go-libp2p-quic-transport"
	swarm "github.com/libp2p/go-libp2p-swarm"
	tnet "github.com/libp2p/go-libp2p-testing/net"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	yamux "github.com/libp2p/go-libp2p-yamux"
	msmux "github.com/libp2p/go-stream-muxer-multistream"
	"github.com/libp2p/go-tcp-transport"

	ma "github.com/multiformats/go-multiaddr"
)

type config struct {
	disableReuseport bool
	dialOnly         bool
	disableTCP       bool
	disableQUIC      bool
	connectionGater  connmgr.ConnectionGater
	sk               crypto.PrivKey
}

// Option is an option that can be passed when constructing a test swarm.
type Option func(*testing.T, *config)

// OptDisableReuseport disables reuseport in this test swarm.
var OptDisableReuseport Option = func(_ *testing.T, c *config) {
	c.disableReuseport = true
}

// OptDialOnly prevents the test swarm from listening.
var OptDialOnly Option = func(_ *testing.T, c *config) {
	c.dialOnly = true
}

// OptDisableTCP disables TCP.
var OptDisableTCP Option = func(_ *testing.T, c *config) {
	c.disableTCP = true
}

// OptDisableQUIC disables QUIC.
var OptDisableQUIC Option = func(_ *testing.T, c *config) {
	c.disableQUIC = true
}

// OptConnGater configures the given connection gater on the test
func OptConnGater(cg connmgr.ConnectionGater) Option {
	return func(_ *testing.T, c *config) {
		c.connectionGater = cg
	}
}

// OptPeerPrivateKey configures the peer private key which is then used to derive the public key and peer ID.
func OptPeerPrivateKey(sk crypto.PrivKey) Option {
	return func(_ *testing.T, c *config) {
		c.sk = sk
	}
}

// GenUpgrader creates a new connection upgrader for use with this swarm.
func GenUpgrader(n *swarm.Swarm) *tptu.Upgrader {
	id := n.LocalPeer()
	pk := n.Peerstore().PrivKey(id)
	secMuxer := new(csms.SSMuxer)
	secMuxer.AddTransport(insecure.ID, insecure.NewWithIdentity(id, pk))

	stMuxer := msmux.NewBlankTransport()
	stMuxer.AddTransport("/yamux/1.0.0", yamux.DefaultTransport)

	return &tptu.Upgrader{
		Secure: secMuxer,
		Muxer:  stMuxer,
	}
}

// GenSwarm generates a new test swarm.
func GenSwarm(t *testing.T, opts ...Option) *swarm.Swarm {
	var cfg config
	for _, o := range opts {
		o(t, &cfg)
	}

	var p tnet.PeerNetParams
	if cfg.sk == nil {
		p = tnet.RandPeerNetParamsOrFatal(t)
	} else {
		pk := cfg.sk.GetPublic()
		id, err := peer.IDFromPublicKey(pk)
		if err != nil {
			t.Fatal(err)
		}
		p.PrivKey = cfg.sk
		p.PubKey = pk
		p.ID = id
		p.Addr = tnet.ZeroLocalTCPAddress
	}

	ps, err := pstoremem.NewPeerstore()
	require.NoError(t, err)
	ps.AddPubKey(p.ID, p.PubKey)
	ps.AddPrivKey(p.ID, p.PrivKey)
	t.Cleanup(func() { ps.Close() })

	swarmOpts := []swarm.Option{swarm.WithMetrics(metrics.NewBandwidthCounter())}
	if cfg.connectionGater != nil {
		swarmOpts = append(swarmOpts, swarm.WithConnectionGater(cfg.connectionGater))
	}
	s, err := swarm.NewSwarm(p.ID, ps, swarmOpts...)
	require.NoError(t, err)

	upgrader := GenUpgrader(s)
	upgrader.ConnGater = cfg.connectionGater

	if !cfg.disableTCP {
		var tcpOpts []tcp.Option
		if cfg.disableReuseport {
			tcpOpts = append(tcpOpts, tcp.DisableReuseport())
		}
		tcpTransport, err := tcp.NewTCPTransport(upgrader, tcpOpts...)
		require.NoError(t, err)
		if err := s.AddTransport(tcpTransport); err != nil {
			t.Fatal(err)
		}
		if !cfg.dialOnly {
			if err := s.Listen(p.Addr); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !cfg.disableQUIC {
		quicTransport, err := quic.NewTransport(p.PrivKey, nil, cfg.connectionGater)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AddTransport(quicTransport); err != nil {
			t.Fatal(err)
		}
		if !cfg.dialOnly {
			if err := s.Listen(ma.StringCast("/ip4/127.0.0.1/udp/0/quic")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !cfg.dialOnly {
		s.Peerstore().AddAddrs(p.ID, s.ListenAddresses(), peerstore.PermanentAddrTTL)
	}
	return s
}

// DivulgeAddresses adds swarm a's addresses to swarm b's peerstore.
func DivulgeAddresses(a, b network.Network) {
	id := a.LocalPeer()
	addrs := a.Peerstore().Addrs(id)
	b.Peerstore().AddAddrs(id, addrs, peerstore.PermanentAddrTTL)
}

// MockConnectionGater is a mock connection gater to be used by the tests.
type MockConnectionGater struct {
	Dial     func(p peer.ID, addr ma.Multiaddr) bool
	PeerDial func(p peer.ID) bool
	Accept   func(c network.ConnMultiaddrs) bool
	Secured  func(network.Direction, peer.ID, network.ConnMultiaddrs) bool
	Upgraded func(c network.Conn) (bool, control.DisconnectReason)
}

func DefaultMockConnectionGater() *MockConnectionGater {
	m := &MockConnectionGater{}
	m.Dial = func(p peer.ID, addr ma.Multiaddr) bool {
		return true
	}

	m.PeerDial = func(p peer.ID) bool {
		return true
	}

	m.Accept = func(c network.ConnMultiaddrs) bool {
		return true
	}

	m.Secured = func(network.Direction, peer.ID, network.ConnMultiaddrs) bool {
		return true
	}

	m.Upgraded = func(c network.Conn) (bool, control.DisconnectReason) {
		return true, 0
	}

	return m
}

func (m *MockConnectionGater) InterceptAddrDial(p peer.ID, addr ma.Multiaddr) (allow bool) {
	return m.Dial(p, addr)
}

func (m *MockConnectionGater) InterceptPeerDial(p peer.ID) (allow bool) {
	return m.PeerDial(p)
}

func (m *MockConnectionGater) InterceptAccept(c network.ConnMultiaddrs) (allow bool) {
	return m.Accept(c)
}

func (m *MockConnectionGater) InterceptSecured(d network.Direction, p peer.ID, c network.ConnMultiaddrs) (allow bool) {
	return m.Secured(d, p, c)
}

func (m *MockConnectionGater) InterceptUpgraded(tc network.Conn) (allow bool, reason control.DisconnectReason) {
	return m.Upgraded(tc)
}
