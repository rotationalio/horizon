package horizon_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/semver"
)

// TestNormalizeCollections verifies that duplicate agents and environments are
// merged by identity, releases are deduplicated, and the resulting collections
// are sorted deterministically for stable API responses.
func TestNormalizeCollections(t *testing.T) {
	agentID := ulid.Make()
	environmentID := ulid.Make()
	firstRelease := horizon.Release{
		ID:      ulid.Make(),
		Slug:    "task",
		Version: semver.MustParse("1.0.0"),
	}
	secondRelease := horizon.Release{
		ID:      ulid.Make(),
		Slug:    "task",
		Version: semver.MustParse("2.0.0"),
	}

	agents := horizon.Agents{
		{
			ID:   agentID,
			Slug: "agent",
			Environments: horizon.Environments{
				{
					ID:   environmentID,
					Slug: "production",
					Tasks: []horizon.Release{
						secondRelease,
					},
				},
			},
		},
		{
			ID:   agentID,
			Slug: "agent",
			Environments: horizon.Environments{
				{
					ID:   environmentID,
					Slug: "production",
					Tasks: []horizon.Release{
						firstRelease,
						secondRelease,
					},
				},
			},
		},
	}

	agents.Normalize()

	require.Len(t, agents, 1)
	require.Len(t, agents[0].Environments, 1)
	require.Len(t, agents[0].Environments[0].Tasks, 2)
	require.Equal(t, firstRelease, agents[0].Environments[0].Tasks[0])
	require.Equal(t, secondRelease, agents[0].Environments[0].Tasks[1])
}
