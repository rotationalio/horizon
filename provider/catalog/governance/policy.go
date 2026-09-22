package governance

import (
	"encoding/json"

	"go.rtnl.ai/ulid"
)

// Policy is a governance filter applied when fetching a provider catalog.
type Policy struct {
	ID          ulid.ULID       `json:"id"`
	Name        string          `json:"name"`
	PolicyType  PolicyType      `json:"policy_type"`
	Constraints json.RawMessage `json:"constraints"`
}
