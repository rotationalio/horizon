package mock

import (
	"context"

	"go.rtnl.ai/endeavor/pkg/errors"
	"go.rtnl.ai/endeavor/pkg/horizon"
)

const Generate = "Generate"

// MockInference is a mock inference client for testing provider clients.
type MockInference struct {
	Mock
	OnGenerate func(ctx context.Context, req *horizon.Request) (*horizon.Response, error)
}

// NewInference creates a new mock inference client.
func NewInference() *MockInference {
	return &MockInference{}
}

func (m *MockInference) Generate(ctx context.Context, req *horizon.Request) (*horizon.Response, error) {
	m.call(Generate)
	if m.OnGenerate != nil {
		return m.OnGenerate(ctx, req)
	}
	panic(errors.Fmt("%s callback is not mocked", Generate))
}
