package mock_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/mock"
)

// Ensures that the mock provider works as expected
func TestMockProvider(t *testing.T) {
	m := mock.New()
	ctx := context.Background()
	m.OnGenerate = func(ctx context.Context, req *provider.Request) (*provider.Response, error) {
		require.Equal(t, context.Background(), ctx)
		require.Nil(t, req)
		return nil, nil
	}
	m.OnFetchCatalog = func(ctx context.Context) ([]catalog.Model, error) {
		require.Equal(t, context.Background(), ctx)
		return nil, nil
	}
	m.OnRetrieveModel = func(ctx context.Context, modelID string) (*catalog.Model, error) {
		require.Equal(t, context.Background(), ctx)
		require.Empty(t, modelID)
		return nil, nil
	}
	m.OnCheckConnectivity = func(ctx context.Context) (provider.Status, error) {
		require.Equal(t, context.Background(), ctx)
		return 0, nil
	}

	// No calls
	require.Equal(t, 0, m.Calls(t, mock.Generate))
	m.AssertNotCalled(t, mock.Generate)
	require.Equal(t, 0, m.Calls(t, mock.FetchCatalog))
	m.AssertNotCalled(t, mock.FetchCatalog)
	require.Equal(t, 0, m.Calls(t, mock.RetrieveModel))
	m.AssertNotCalled(t, mock.RetrieveModel)
	require.Equal(t, 0, m.Calls(t, mock.CheckConnectivity))
	m.AssertNotCalled(t, mock.CheckConnectivity)

	// Generate
	res, err := m.Generate(ctx, nil)
	require.Nil(t, res)
	require.Nil(t, err)
	require.Equal(t, 1, m.Calls(t, mock.Generate))
	m.AssertCalled(t, mock.Generate, 1)

	// FetchCatalog
	catalog, err := m.FetchCatalog(ctx)
	require.Nil(t, catalog)
	require.Nil(t, err)
	require.Equal(t, 1, m.Calls(t, mock.FetchCatalog))
	m.AssertCalled(t, mock.FetchCatalog, 1)

	// RetrieveModel
	model, err := m.RetrieveModel(ctx, "")
	require.Nil(t, model)
	require.Nil(t, err)
	require.Equal(t, 1, m.Calls(t, mock.RetrieveModel))
	m.AssertCalled(t, mock.RetrieveModel, 1)

	// CheckConnectivity
	status, err := m.CheckConnectivity(ctx)
	require.Equal(t, provider.StatusUnknown, status)
	require.Nil(t, err)
	require.Equal(t, 1, m.Calls(t, mock.CheckConnectivity))
	m.AssertCalled(t, mock.CheckConnectivity, 1)

	// Reset - no calls
	m.Reset()
	require.Equal(t, 0, m.Calls(t, mock.Generate))
	m.AssertNotCalled(t, mock.Generate)
	require.Equal(t, 0, m.Calls(t, mock.FetchCatalog))
	m.AssertNotCalled(t, mock.FetchCatalog)
	require.Equal(t, 0, m.Calls(t, mock.RetrieveModel))
	m.AssertNotCalled(t, mock.RetrieveModel)
	require.Equal(t, 0, m.Calls(t, mock.CheckConnectivity))
	m.AssertNotCalled(t, mock.CheckConnectivity)
}
