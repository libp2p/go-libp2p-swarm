module github.com/libp2p/go-libp2p-swarm

require (
	github.com/gogo/protobuf v1.3.1
	github.com/ipfs/go-log v1.0.3
	github.com/jbenet/goprocess v0.1.4
	github.com/libp2p/go-addr-util v0.0.1
	github.com/libp2p/go-conn-security-multistream v0.1.0
	github.com/libp2p/go-libp2p-core v0.5.6
	github.com/libp2p/go-libp2p-loggables v0.1.0
	github.com/libp2p/go-libp2p-peerstore v0.2.4
	github.com/libp2p/go-libp2p-secio v0.2.1
	github.com/libp2p/go-libp2p-testing v0.1.1
	github.com/libp2p/go-libp2p-transport-upgrader v0.2.0
	github.com/libp2p/go-libp2p-yamux v0.2.2
	github.com/libp2p/go-maddr-filter v0.0.5
	github.com/libp2p/go-stream-muxer-multistream v0.2.0
	github.com/libp2p/go-tcp-transport v0.1.1
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-fmt v0.1.0
	github.com/multiformats/go-multiaddr-net v0.1.4
	github.com/multiformats/go-multibase v0.0.2 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37 // indirect
	golang.org/x/sys v0.0.0-20200519105757-fe76b779f299 // indirect
)

replace github.com/libp2p/go-libp2p-core => ../go-libp2p-core

go 1.12
