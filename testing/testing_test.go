package testing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenSwarm(t *testing.T) {
	swarm := GenSwarm(t, context.Background())
	require.NoError(t, swarm.Close())
	GenUpgrader(swarm)
}
