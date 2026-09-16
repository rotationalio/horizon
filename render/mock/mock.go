package mock

// ============================================================================
// Mock Helper
// ============================================================================

// Mock helper for testing the render package.
// TODO: try to merge this with the mock package's [Mock] type
type Mock struct {
	Err   error // If no callback function is specified, this error will be returned
	calls int   // The number of times the mock has been called
}

func (m *Mock) Calls() int {
	return m.calls
}

func (r *Mock) Called(expected int) bool {
	return r.calls == expected
}

func (m *Mock) Reset() {
	m.calls = 0
}

// ============================================================================
// Mock Renderer
// ============================================================================

type Renderer struct {
	Mock
	OnRender func(data map[string]any) (string, error)
}

func (r *Renderer) Render(data map[string]any) (string, error) {
	r.calls++
	if r.OnRender != nil {
		return r.OnRender(data)
	}
	return "", r.Err
}

func RenderErr(err error) *Renderer {
	return &Renderer{Mock: Mock{Err: err}}
}
