// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/p2p/net/swarm/testing.
package testing

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	swarm_testing "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	tptu "github.com/libp2p/go-libp2p/p2p/net/upgrader"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/transport"
)

// Option is an option that can be passed when constructing a test swarm.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.Option instead.
type Option = swarm_testing.Option

// OptDisableReuseport disables reuseport in this test swarm.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptDisableReuseport instead.
var OptDisableReuseport Option = swarm_testing.OptDisableReuseport

// OptDialOnly prevents the test swarm from listening.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptDialOnly instead.
var OptDialOnly Option = swarm_testing.OptDialOnly

// OptDisableTCP disables TCP.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptDisableTCP instead.
var OptDisableTCP Option = swarm_testing.OptDisableTCP

// OptDisableQUIC disables QUIC.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptDisableQUIC instead.
var OptDisableQUIC Option = swarm_testing.OptDisableQUIC

// OptConnGater configures the given connection gater on the test
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptConnGater instead.
func OptConnGater(cg connmgr.ConnectionGater) Option {
	return swarm_testing.OptConnGater(cg)
}

// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptResourceManager instead.
func OptResourceManager(rcmgr network.ResourceManager) Option {
	return swarm_testing.OptResourceManager(rcmgr)
}

// OptPeerPrivateKey configures the peer private key which is then used to derive the public key and peer ID.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.OptPeerPrivateKey instead.
func OptPeerPrivateKey(sk crypto.PrivKey) Option {
	return swarm_testing.OptPeerPrivateKey(sk)
}

// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.DialTimeout instead.
func DialTimeout(t time.Duration) Option {
	return swarm_testing.DialTimeout(t)
}

// GenUpgrader creates a new connection upgrader for use with this swarm.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.GenUpgrader instead.
func GenUpgrader(t *testing.T, n *swarm.Swarm, opts ...tptu.Option) transport.Upgrader {
	return swarm_testing.GenUpgrader(t, n, opts...)
}

// GenSwarm generates a new test swarm.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.GenSwarm instead.
func GenSwarm(t *testing.T, opts ...Option) *swarm.Swarm {
	return swarm_testing.GenSwarm(t, opts...)
}

// DivulgeAddresses adds swarm a's addresses to swarm b's peerstore.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.DivulgeAddresses instead.
func DivulgeAddresses(a, b network.Network) {
	swarm_testing.DivulgeAddresses(a, b)
}

// MockConnectionGater is a mock connection gater to be used by the tests.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/net/swarm/testing.MockConnectionGater instead.
type MockConnectionGater = swarm_testing.MockConnectionGater
