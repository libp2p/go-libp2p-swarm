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
	disableReuseport        bool
	dialOnly                bool
	disableTCP              bool
	disableQUIC             bool
	useHandshakeNegotiation bool
	connectionGater         connmgr.ConnectionGater
	sk                      crypto.PrivKey
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

func OptUseHandshakeNegotiation() Option {
	return func(_ *testing.T, c *config) {
		c.useHandshakeNegotiation = true
	}
}

type upgraderConf struct {
	useHandshakeNegotiation bool
}

type UpgraderOption func(*upgraderConf)

func UseHandshakeNegotiation() UpgraderOption {
	return func(conf *upgraderConf) {
		conf.useHandshakeNegotiation = true
	}
}

// GenUpgrader creates a new connection upgrader for use with this swarm.
func GenUpgrader(n *swarm.Swarm, opts ...UpgraderOption) *tptu.Upgrader {
	var conf upgraderConf
	for _, o := range opts {
		o(&conf)
	}
	id := n.LocalPeer()
	pk := n.Peerstore().PrivKey(id)

	stMuxer := msmux.NewBlankTransport()
	stMuxer.AddTransport("/yamux/1.0.0", yamux.DefaultTransport)
	upgrader := &tptu.Upgrader{Muxer: stMuxer}

	if conf.useHandshakeNegotiation {
		secMuxer := new(csms.SSMuxer)
		secMuxer.AddTransport(insecure.ID, insecure.NewWithIdentity(id, pk))
		upgrader.SecureMuxer = secMuxer
	} else {
		upgrader.SecureTransport = insecure.NewWithIdentity(id, pk)
	}
	return upgrader
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
		require.NoError(t, err)
		p.PrivKey = cfg.sk
		p.PubKey = pk
		p.ID = id
		p.Addr = tnet.ZeroLocalTCPAddress
	}

	ps := pstoremem.NewPeerstore()
	ps.AddPubKey(p.ID, p.PubKey)
	ps.AddPrivKey(p.ID, p.PrivKey)
	t.Cleanup(func() { ps.Close() })

	swarmOpts := []swarm.Option{swarm.WithMetrics(metrics.NewBandwidthCounter())}
	if cfg.connectionGater != nil {
		swarmOpts = append(swarmOpts, swarm.WithConnectionGater(cfg.connectionGater))
	}
	s, err := swarm.NewSwarm(p.ID, ps, swarmOpts...)
	require.NoError(t, err)

	var upgraderOpts []UpgraderOption
	if cfg.useHandshakeNegotiation {
		upgraderOpts = append(upgraderOpts, UseHandshakeNegotiation())
	}
	upgrader := GenUpgrader(s, upgraderOpts...)
	upgrader.ConnGater = cfg.connectionGater
	if upgrader.SecureTransport != nil {
		p.Addr = p.Addr.Encapsulate(ma.StringCast("/" + upgrader.SecureTransport.Protocol().Name))
	}

	if !cfg.disableTCP {
		tcpTransport := tcp.NewTCPTransport(upgrader)
		tcpTransport.DisableReuseport = cfg.disableReuseport
		require.NoError(t, s.AddTransport(tcpTransport))
		if !cfg.dialOnly {
			require.NoError(t, s.Listen(p.Addr))
		}
	}
	if !cfg.disableQUIC {
		quicTransport, err := quic.NewTransport(p.PrivKey, nil, cfg.connectionGater)
		require.NoError(t, err)
		require.NoError(t, s.AddTransport(quicTransport))
		if !cfg.dialOnly {
			require.NoError(t, s.Listen(ma.StringCast("/ip4/127.0.0.1/udp/0/quic")))
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
