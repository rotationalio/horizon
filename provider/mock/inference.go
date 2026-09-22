package mock

import (
	"context"

	"go.rtnl.ai/horizon/errors"
	api "go.rtnl.ai/horizon/provider/api"
)

const Generate = "Generate"

// MockInference is a mock inference client for testing provider clients.
type MockInference struct {
	Mock
	OnGenerate func(ctx context.Context, req *api.Request) (*api.Response, error)
}

// NewInference creates a new mock inference client.
func NewInference() *MockInference {
	return &MockInference{}
}

func (m *MockInference) Generate(ctx context.Context, req *api.Request) (*api.Response, error) {
	m.call(Generate)
	if m.OnGenerate != nil {
		return m.OnGenerate(ctx, req)
	}
	panic(errors.Fmt("%s callback is not mocked", Generate))
}
