package swarm

import (
	"github.com/libp2p/go-libp2p-swarm/dial"
)

type Option func(*Swarm) error

// WithDialPipeline injects a custom dial pipeline in this swarm.
//
// To modify behaviour from the default pipeline, use NewDefaultPipeline and the accessor methods on *dial.Pipeline:
//
//  factory := func(s *swarm.Swarm) *dial.Pipeline {
//		pipeline := s.NewDefaultPipeline()
//
//  	var planner dial.Planner
//  	pipeline.SetPlanner(planner)
//
//  	prep := pipeline.Preparer()
//  	seq := prep.(*dial.PreparerSeq)
//
//  	var newBackoff Preparer
//  	// replace the backokff preparer.
//  	seq.Replace("backoff", newBackoff)
//
//		return pipeline
//  }
//
//  s := swarm.NewSwarm(ctx, pid, peerstore, swarm.WithDialPipeline(factory))
func WithDialPipeline(factory func(swarm *Swarm) *dial.Pipeline) Option {
	return func(s *Swarm) error {
		s.pipeline = factory(s)
		return nil
	}
}
