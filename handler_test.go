package horizon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/attachments"
	providermock "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
)

type inputRunner struct {
	*mock.Runner
	onInput func(*task.Input) (*task.Input, error)
}

func (r *inputRunner) ProcessInput(input *task.Input) (*task.Input, error) {
	return r.onInput(input)
}

// Verifies routed requests preserve GET, JSON, multipart input, and output handling.
func TestHandler(t *testing.T) {
	tests := []struct {
		name        string
		request     func(*testing.T) *http.Request
		attachments bool
	}{
		{
			name: "Get",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/task?context=value", nil)
			},
		},
		{
			name: "Post",
			request: func(t *testing.T) *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(`{"context":"value"}`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
		},
		{
			name:        "Multipart",
			request:     multipartRequest,
			attachments: true,
		},
		{
			name: "ReplyAttachments",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/task?context=value", nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, processed := 0, 0
			h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
				created++
				runner := selectedRunner()
				runner.OnFinalize = func(_ context.Context, output *task.Output) error {
					output.Output = "Hello, world!"
					if tt.name == "ReplyAttachments" {
						output.Attachments = attachments.Attachments{{Filename: "test.txt", Data: []byte("Generated file content.")}}
					}
					return nil
				}
				return &inputRunner{Runner: runner, onInput: func(input *task.Input) (*task.Input, error) {
					processed++
					require.Equal(t, "value", input.Context["context"])
					if tt.attachments {
						require.Len(t, input.Attachments, 1)
						require.Equal(t, "test.txt", input.Attachments[0].Filename)
						require.Equal(t, []byte("This is an attached file."), input.Attachments[0].Data)
					}
					return input, nil
				}}, nil
			}))
			require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
			tsk := &task.Task{Output: &schema.Output{}}
			handler := h.Handler()
			handler.Router().Insert("/task", tsk)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, tt.request(t))

			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, 1, created)
			require.Equal(t, 1, processed)
			require.Nil(t, tsk.Provider)
			if tt.name == "ReplyAttachments" {
				mediaType, params, err := mime.ParseMediaType(response.Header().Get("Content-Type"))
				require.NoError(t, err)
				require.Equal(t, "multipart/form-data", mediaType)
				reader := multipart.NewReader(response.Body, params["boundary"])
				part, err := reader.NextPart()
				require.NoError(t, err)
				require.Equal(t, "output", part.FormName())
				var output task.Output
				require.NoError(t, json.NewDecoder(part).Decode(&output))
				require.Equal(t, "Hello, world!", output.Output)
				part, err = reader.NextPart()
				require.NoError(t, err)
				require.Equal(t, "test.txt", part.FileName())
				data, err := io.ReadAll(part)
				require.NoError(t, err)
				require.Equal(t, "Generated file content.", string(data))
				_, err = reader.NextPart()
				require.ErrorIs(t, err, io.EOF)
			} else {
				require.Equal(t, "application/json", response.Header().Get("Content-Type"))
				var output task.Output
				require.NoError(t, json.NewDecoder(response.Body).Decode(&output))
				require.Equal(t, "Hello, world!", output.Output)
			}
		})
	}
}

// Verifies route and input failures do not allocate runners, while execution
// failures produce JSON error responses through the same handler.
func TestHandlerErrors(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		body        string
		factoryErr  bool
		prepareErr  bool
		finalizeErr bool
		status      int
		created     int
	}{
		{
			name:   "NotFound",
			path:   "/missing",
			body:   `{}`,
			status: http.StatusNotFound,
		},
		{
			name:   "BadRequest",
			path:   "/task",
			body:   `invalid`,
			status: http.StatusBadRequest,
		},
		{
			name:       "Factory",
			path:       "/task",
			body:       `{}`,
			factoryErr: true,
			status:     http.StatusInternalServerError,
			created:    1,
		},
		{
			name:       "Prepare",
			path:       "/task",
			body:       `{}`,
			prepareErr: true,
			status:     http.StatusInternalServerError,
			created:    1,
		},
		{
			name:        "Finalize",
			path:        "/task",
			body:        `{}`,
			finalizeErr: true,
			status:      http.StatusInternalServerError,
			created:     1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := 0
			h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
				created++
				if tt.factoryErr {
					return nil, errors.New("factory failed")
				}
				runner := selectedRunner()
				if tt.prepareErr {
					runner.OnPrepare = func(context.Context, *task.Task) error { return errors.New("prepare failed") }
				}
				if tt.finalizeErr {
					runner.OnFinalize = func(context.Context, *task.Output) error { return errors.New("finalize failed") }
				}
				return runner, nil
			}))
			require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
			h.Handler().Router().Insert("/task", &task.Task{Output: &schema.Output{}})
			response := httptest.NewRecorder()
			h.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))
			require.Equal(t, tt.status, response.Code)
			require.Equal(t, "application/json", response.Header().Get("Content-Type"))
			require.Equal(t, tt.created, created)
			require.True(t, json.Valid(response.Body.Bytes()))
		})
	}
}

// Verifies callers can insert, replace, retrieve, and remove routes on Horizon's handler.
func TestHandlerRoutes(t *testing.T) {
	h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
		runner := selectedRunner()
		var name string
		prepare := runner.OnPrepare
		runner.OnPrepare = func(ctx context.Context, tsk *task.Task) error {
			name = tsk.Name
			return prepare(ctx, tsk)
		}
		runner.OnFinalize = func(_ context.Context, output *task.Output) error {
			output.Output = name
			return nil
		}
		return runner, nil
	}))
	require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
	handler := h.Handler()
	first := &task.Task{Name: "first", Output: &schema.Output{}}
	require.False(t, handler.Router().Insert("/task", first))
	require.Equal(t, 1, handler.Router().Size())
	found, ok := handler.Router().Get("/task")
	require.True(t, ok)
	require.Same(t, first, found)
	for _, name := range []string{"first", "replacement"} {
		if name == "replacement" {
			require.True(t, handler.Router().Insert("/task", &task.Task{Name: name, Output: &schema.Output{}}))
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/task", nil))
		require.Equal(t, http.StatusOK, response.Code)
		var output task.Output
		require.NoError(t, json.NewDecoder(response.Body).Decode(&output))
		require.Equal(t, name, output.Output)
	}
	require.True(t, handler.Router().Remove("/task"))
	require.False(t, handler.Router().Remove("/task"))
	require.Zero(t, handler.Router().Size())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/task", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
}

// Verifies the handler can be mounted directly on a standard-library HTTP mux.
func TestHandlerServeMux(t *testing.T) {
	h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
		return &mock.Runner{}, nil
	}))
	config := testProviderConfig()
	require.NoError(t, h.AddProviderInstance(config, providermock.New(testProviderID)))
	h.Handler().Router().Insert("/task", &task.Task{
		Provider: &config,
		Output:   &schema.Output{},
	})
	mux := http.NewServeMux()
	mux.Handle("/", h.Handler())
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/task", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
}

// Verifies simultaneous HTTP executions have isolated runner state while routes change.
func TestHandlerConcurrent(t *testing.T) {
	const requests = 12
	var created atomic.Int64
	h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
		id := created.Add(1)
		runner := selectedRunner()
		runner.OnFinalize = func(_ context.Context, output *task.Output) error {
			output.Output = id
			return nil
		}
		return runner, nil
	}))
	require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
	handler := h.Handler()
	first := &task.Task{Output: &schema.Output{}}
	second := &task.Task{Name: "replacement", Output: &schema.Output{}}
	handler.Router().Insert("/task", first)
	responses := make(chan *httptest.ResponseRecorder, requests)
	updated := make(chan struct{})
	go func() {
		defer close(updated)
		for range requests {
			handler.Router().Insert("/task", second)
			handler.Router().Insert("/task", first)
		}
	}()
	for range requests {
		go func() {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/task", nil))
			responses <- response
		}()
	}
	ids := make(map[float64]bool)
	for range requests {
		response := <-responses
		require.Equal(t, http.StatusOK, response.Code)
		var output task.Output
		require.NoError(t, json.NewDecoder(response.Body).Decode(&output))
		id, ok := output.Output.(float64)
		require.True(t, ok)
		ids[id] = true
	}
	<-updated
	require.Len(t, ids, requests)
	require.EqualValues(t, requests, created.Load())
	require.Nil(t, first.Provider)
	require.Nil(t, second.Provider)
}

// Builds multipart input containing context and one file attachment.
func multipartRequest(t *testing.T) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("context", `"value"`))
	part, err := writer.CreateFormFile("file", "test.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("This is an attached file."))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/task", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
