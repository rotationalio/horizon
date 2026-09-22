package mock

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Helper for creating mock objects for testing that tracks method call counts.
// Embed this type in your mock type and use [Mock.call] to record a method
// call, [Mock.AssertCalled] to assert that a method was called the expected
// number of times, and [Mock.AssertNotCalled] to assert that a method was not
// called.
type Mock struct {
	callMu sync.RWMutex
	calls  map[string]int
}

// Records a method call.
func (m *Mock) call(op string) {
	m.callMu.Lock()
	defer m.callMu.Unlock()
	if m.calls == nil {
		m.calls = make(map[string]int)
	}
	m.calls[op]++
}

// Asserts that a method was called the expected number of times.
func (m *Mock) AssertCalled(t testing.TB, op string, expected int) {
	t.Helper()
	m.callMu.RLock()
	defer m.callMu.RUnlock()
	require.NotNil(t, m.calls, "mock has not been called at all")
	require.Equal(t, expected, m.calls[op], "expected mock operation %s to be called %d times, got %d", op, expected, m.calls[op])
}

// Asserts that a method was not called.
func (m *Mock) AssertNotCalled(t testing.TB, op string) {
	t.Helper()
	m.callMu.RLock()
	defer m.callMu.RUnlock()

	require.True(t, m.calls == nil || m.calls[op] == 0, "expected mock operation %s to not be called")
}
