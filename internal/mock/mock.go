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
//
// NOTE: This will serialize method calls using a mutex, so concurrency testing
// should probably use a different approach for mocking.
type Mock struct {
	callMu sync.RWMutex
	calls  map[string]int
}

// Resets the call count for all methods. Takes a write lock on the call map
// mutex.
func (m *Mock) Reset() {
	m.callMu.Lock()
	defer m.callMu.Unlock()
	m.calls = nil
}

// Records a method call. Takes a write lock on the call map mutex.
func (m *Mock) Call(method string) {
	m.callMu.Lock()
	defer m.callMu.Unlock()
	if m.calls == nil {
		m.calls = make(map[string]int)
	}
	m.calls[method]++
}

// Returns the method's call count. Takes a read lock on the call map mutex.
func (m *Mock) Calls(t testing.TB, method string) int {
	t.Helper()
	m.callMu.RLock()
	defer m.callMu.RUnlock()
	if m.calls == nil {
		return 0
	}
	return m.calls[method]
}

// Asserts that a method was called the expected number of times. Takes a read
// lock on the call map mutex.
func (m *Mock) AssertCalled(t testing.TB, method string, expected int) {
	t.Helper()
	m.callMu.RLock()
	defer m.callMu.RUnlock()
	require.NotNil(t, m.calls, "mock has not been called at all")
	require.Equal(t, expected, m.calls[method], "expected mock operation %s to be called %d times, got %d", method, expected, m.calls[method])
}

// Asserts that a method was not called. Takes a read lock on the call map mutex.
func (m *Mock) AssertNotCalled(t testing.TB, method string) {
	t.Helper()
	m.callMu.RLock()
	defer m.callMu.RUnlock()
	require.True(t, m.calls == nil || m.calls[method] == 0, "expected mock operation %s to not be called (called %d times)", method, m.calls[method])
}
