package mock_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/internal/mock"
)

func TestMock(t *testing.T) {
	m := &mock.Mock{}
	require.Equal(t, 0, m.Calls(t, "foo"))
	m.AssertNotCalled(t, "foo")

	m.Call("foo")
	require.Equal(t, 1, m.Calls(t, "foo"))
	m.AssertCalled(t, "foo", 1)
	m.AssertNotCalled(t, "bar")

	m.Reset()
	require.Equal(t, 0, m.Calls(t, "foo"))
	m.AssertNotCalled(t, "foo")
}
