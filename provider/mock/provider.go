package mock

import (
	"context"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/internal/mock"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/catalog"
)

const (
	Generate          = "Generate"
	FetchCatalog      = "FetchCatalog"
	RetrieveModel     = "RetrieveModel"
	CheckConnectivity = "CheckConnectivity"

	ConnectivityModel = "mock/connectivity"
)

type MockProvider struct {
	mock.Mock
	OnGenerate          func(ctx context.Context, req *provider.Request) (*provider.Response, error)
	OnFetchCatalog      func(ctx context.Context) ([]catalog.Model, error)
	OnRetrieveModel     func(ctx context.Context, modelID string) (*catalog.Model, error)
	OnCheckConnectivity func(ctx context.Context) (provider.Status, error)
}

var _ provider.Provider = (*MockProvider)(nil)

func New() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Generate(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	m.Call(Generate)
	if m.OnGenerate != nil {
		return m.OnGenerate(ctx, req)
	}
	panic(errors.Fmt("%s callback is not mocked", Generate))
}

func (m *MockProvider) FetchCatalog(ctx context.Context) ([]catalog.Model, error) {
	m.Call(FetchCatalog)
	if m.OnFetchCatalog != nil {
		return m.OnFetchCatalog(ctx)
	}
	panic(errors.Fmt("%s callback is not mocked", FetchCatalog))
}

func (m *MockProvider) RetrieveModel(ctx context.Context, modelID string) (*catalog.Model, error) {
	m.Call(RetrieveModel)
	if m.OnRetrieveModel != nil {
		return m.OnRetrieveModel(ctx, modelID)
	}
	panic(errors.Fmt("%s callback is not mocked", RetrieveModel))
}

func (m *MockProvider) CheckConnectivity(ctx context.Context) (provider.Status, error) {
	m.Call(CheckConnectivity)
	if m.OnCheckConnectivity != nil {
		return m.OnCheckConnectivity(ctx)
	}
	panic(errors.Fmt("%s callback is not mocked", CheckConnectivity))
}
