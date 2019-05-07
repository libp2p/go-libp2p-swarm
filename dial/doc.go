// Package dial contains the logic to establish outbound connections to other peers. In go lingo, that process is
// called "dialing".
//
// The central component of this package is the dial Pipeline. A Pipeline assembles a set of modular components,
// each with a clearly-delimited responsibility, into a dialing engine. Consumers can replace, delete, add components
// to customize the dialing engine's behavior.
//
// The Pipeline comprises five components. We provide brief descriptions below. For more detail, refer to the
// respective godocs of each interface:
//
//   - Preparer: runs preparatory actions prior to the dial execution. Actions may include: deduplicating, populating
//     timeouts, circuit breaking, validating the peer ID, etc.
//   - AddressResolver: populates the set of addresses for a peer, either from the peerstore and/or from other sources,
//     such as a discovery process.
//   - Planner: schedules dials in time, responding to events from the environment, such as new addresses discovered,
//     or dial jobs completing.
//   - Throttler: throttles dials based on resource usage or other factors.
//   - Executor: actually carries out the network dials.
//
// This package provides basic implementations of all five dialer components, as well as a default Pipeline suitable
// for simple host constructions. See the godocs on NewDefaultPipeline for details of the composition of the
// default Pipeline. Note that the user can customize the Pipeline using the methods in that struct.
//
// These five components deal with two main entities: the Request and the Job. A Request represents a caller request
// to dial a peer, identified by a peer ID. Along the execution of the Pipeline, the Request will translate into
// one or many dial Jobs, each of which targets a multiaddr where that peer ID is presumably listening.
package dial
