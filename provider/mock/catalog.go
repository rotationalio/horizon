package mock

import (
	"context"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
)

const (
	Fetch    = "Fetch"
	Retrieve = "Retrieve"
)

// MockCatalog is a mock catalog client for testing provider clients.
type MockCatalog struct {
	Mock
	OnFetch    func(ctx context.Context, policies ...governance.Policy) ([]catalog.Model, error)
	OnRetrieve func(ctx context.Context, modelID string, policies ...governance.Policy) (*catalog.Model, error)
}

// NewCatalog creates a new mock catalog client.
func NewCatalog() *MockCatalog {
	return &MockCatalog{}
}

func (m *MockCatalog) Fetch(ctx context.Context, policies ...governance.Policy) ([]catalog.Model, error) {
	m.call(Fetch)
	if m.OnFetch != nil {
		return m.OnFetch(ctx, policies...)
	}
	panic(errors.Fmt("%s callback is not mocked", Fetch))
}

func (m *MockCatalog) Retrieve(ctx context.Context, modelID string, policies ...governance.Policy) (*catalog.Model, error) {
	m.call(Retrieve)
	if m.OnRetrieve != nil {
		return m.OnRetrieve(ctx, modelID, policies...)
	}
	panic(errors.Fmt("%s callback is not mocked", Retrieve))
}
