package horizon

import (
	"slices"

	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/region"
	"go.rtnl.ai/x/semver"
)

//============================================================================
// Agent
//============================================================================

// A logical grouping of tasks within execution environments.
type Agent struct {
	ID           ulid.ULID    `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"`                            // The database-specific ID of the agent
	Slug         string       `json:"slug" yaml:"slug" msg:"slug"`                                                    // The slug of the agent for the URL component
	Name         string       `json:"name" yaml:"name" msg:"name"`                                                    // The name of the agent
	Description  string       `json:"description,omitempty" yaml:"description,omitempty" msg:"description,omitempty"` // The description of the agent
	Environments Environments `json:"environments" yaml:"environments" msg:"environments"`                            // The environments of the agent
}

//============================================================================
// Environment
//============================================================================

// A logical grouping of tasks for a specific execution environment.
type Environment struct {
	ID      ulid.ULID      `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"`            // the ID of the environment
	Slug    string         `json:"slug,omitempty" yaml:"slug,omitempty" msg:"slug,omitempty"`      // the slug of the environment
	Name    string         `json:"name" yaml:"name" msg:"name"`                                    // production, staging, development, etc.
	Version semver.Version `json:"version" yaml:"version" msg:"version"`                           // the current, global version of the environment
	Region  region.Info    `json:"region,omitzero" yaml:"region,omitempty" msg:"region,omitempty"` // the region of the environment (only regionID and cluster need be specified)
	Tasks   []Release      `json:"tasks" yaml:"tasks" msg:"tasks"`                                 // the tasks available in the environment and their versions
}

// Removes duplicate Releases and sorts them by ID.
func (e *Environment) Normalize() {
	normalized := make([]Release, 0, len(e.Tasks))
	byID := make(map[ulid.ULID]int, len(e.Tasks))
	bySlugVersion := make(map[string]int, len(e.Tasks))

	for _, release := range e.Tasks {
		found := false
		if !release.ID.IsZero() {
			_, found = byID[release.ID]
		}
		if !found {
			_, found = bySlugVersion[release.Slug+"\x00"+release.Version.String()]
		}
		if found {
			continue
		}

		index := len(normalized)
		normalized = append(normalized, release)
		if !release.ID.IsZero() {
			byID[release.ID] = index
		}
		bySlugVersion[release.Slug+"\x00"+release.Version.String()] = index
	}

	slices.SortStableFunc(normalized, func(left, right Release) int {
		return left.ID.Compare(right.ID)
	})
	e.Tasks = normalized
}

//============================================================================
// Release
//============================================================================

// Release describes a deployable [Task] release.
type Release struct {
	ID      ulid.ULID      `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"` // the ID of the Release
	Slug    string         `json:"slug" yaml:"slug" msg:"slug"`                         // the slug of the Release
	Version semver.Version `json:"version" yaml:"version" msg:"version"`                // the specified version of the task
}

//============================================================================
// Agent Collections
//============================================================================

// A collection of [Agent]s.
type Agents []Agent

// Removes duplicate agents, merges environments, and sorts by ID.
func (a *Agents) Normalize() {
	if a == nil {
		return
	}

	normalized := make(Agents, 0, len(*a))
	indexes := make(map[string]int, len(*a))
	for _, agent := range *a {
		key, identified := collectionIdentity(agent.ID, agent.Slug)
		if identified {
			if index, ok := indexes[key]; ok {
				normalized[index].Environments = append(normalized[index].Environments, agent.Environments...)
				continue
			}
		}

		if identified {
			indexes[key] = len(normalized)
		}
		normalized = append(normalized, agent)
	}

	for i := range normalized {
		normalized[i].Environments.Normalize()
	}
	slices.SortStableFunc(normalized, func(left, right Agent) int {
		return left.ID.Compare(right.ID)
	})
	*a = normalized
}

// A collection of [Environment]s.
type Environments []Environment

// Removes duplicate environments, merges tasks, and sorts by ID.
func (e *Environments) Normalize() {
	if e == nil {
		return
	}

	normalized := make(Environments, 0, len(*e))
	indexes := make(map[string]int, len(*e))
	for _, environment := range *e {
		key, identified := collectionIdentity(environment.ID, environment.Slug)
		if identified {
			if index, ok := indexes[key]; ok {
				normalized[index].Tasks = append(normalized[index].Tasks, environment.Tasks...)
				continue
			}
		}

		if identified {
			indexes[key] = len(normalized)
		}
		normalized = append(normalized, environment)
	}

	for i := range normalized {
		normalized[i].Normalize()
	}
	slices.SortStableFunc(normalized, func(left, right Environment) int {
		return left.ID.Compare(right.ID)
	})
	*e = normalized
}

// Chooses an ID key when available and otherwise falls back to a slug.
func collectionIdentity(id ulid.ULID, slug string) (string, bool) {
	if !id.IsZero() {
		return "id:" + id.String(), true
	}
	if slug != "" {
		return "slug:" + slug, true
	}
	return "", false
}
