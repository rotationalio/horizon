package mock

import (
	"context"

	"go.rtnl.ai/horizon/errors"
	api "go.rtnl.ai/horizon/provider/api"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
	"go.rtnl.ai/horizon/provider/types"
)

const (
	FetchCatalog      = "FetchCatalog"
	RetrieveModel     = "RetrieveModel"
	CheckConnectivity = "CheckConnectivity"

	ConnectivityModel = "mock/connectivity"
)

type MockProvider struct {
	Mock
	OnGenerate          func(ctx context.Context, req *api.Request) (*api.Response, error)
	OnFetchCatalog      func(ctx context.Context, policies ...governance.Policy) ([]catalog.Model, error)
	OnRetrieveModel     func(ctx context.Context, modelID string, policies ...governance.Policy) (*catalog.Model, error)
	OnCheckConnectivity func(ctx context.Context) (types.Status, error)
}

func NewProvider() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Generate(ctx context.Context, req *api.Request) (*api.Response, error) {
	m.call(Generate)
	if m.OnGenerate != nil {
		return m.OnGenerate(ctx, req)
	}
	panic(errors.Fmt("%s callback is not mocked", Generate))
}

func (m *MockProvider) FetchCatalog(ctx context.Context, policies ...governance.Policy) ([]catalog.Model, error) {
	m.call(FetchCatalog)
	if m.OnFetchCatalog != nil {
		return m.OnFetchCatalog(ctx, policies...)
	}
	panic(errors.Fmt("%s callback is not mocked", FetchCatalog))
}

func (m *MockProvider) RetrieveModel(ctx context.Context, modelID string, policies ...governance.Policy) (*catalog.Model, error) {
	m.call(RetrieveModel)
	if m.OnRetrieveModel != nil {
		return m.OnRetrieveModel(ctx, modelID, policies...)
	}
	panic(errors.Fmt("%s callback is not mocked", RetrieveModel))
}

func (m *MockProvider) CheckConnectivity(ctx context.Context) (types.Status, error) {
	m.call(CheckConnectivity)
	if m.OnCheckConnectivity != nil {
		return m.OnCheckConnectivity(ctx)
	}
	panic(errors.Fmt("%s callback is not mocked", CheckConnectivity))
}
