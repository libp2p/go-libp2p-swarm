package swarm

// Option is a Swarm Option that can be given to a Swarm Constructor(`NewSwarm`).
type Option func(s *Swarm)

func (s *Swarm) ApplyOptions(opts ...Option) {
	for _, opt := range opts {
		opt(s)
	}
}

func SwarmPeerLimit(limit int) Option {
	return func(s *Swarm) {
		s.peerLimit = limit
	}
}
