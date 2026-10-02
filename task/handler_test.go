package task_test

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
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
)

// Test making requests to the HTTP handler
func TestTaskHandler(t *testing.T) {
	tsk := &task.Task{
		Output:       &schema.Output{},
		Capabilities: &task.Capabilities{},
	}

	t.Run("Get", func(t *testing.T) {
		runner := &mock.Runner{
			OnFinalize: func(ctx context.Context, output *task.Output) error {
				output.Output = "Hello, world!"
				return nil
			},
		}
		handler := task.NewTaskHandler(tsk, runner)
		request, err := http.NewRequest("GET", "?context=value", nil)
		require.NoError(t, err)
		response := httptest.NewRecorder()

		// Handle the request
		handler.Handle(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var output task.Output
		err = json.NewDecoder(response.Body).Decode(&output)
		require.NoError(t, err)
		require.Equal(t, "Hello, world!", output.Output)
	})

	t.Run("Post", func(t *testing.T) {
		runner := &mock.Runner{
			OnFinalize: func(ctx context.Context, output *task.Output) error {
				output.Output = "Hello, world!"
				return nil
			},
		}
		handler := task.NewTaskHandler(tsk, runner)
		data := map[string]any{
			"context": "value",
		}
		body, err := json.Marshal(data)
		require.NoError(t, err)
		request, err := http.NewRequest("POST", "", bytes.NewBuffer(body))
		require.NoError(t, err)
		response := httptest.NewRecorder()

		// Handle the request
		handler.Handle(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var output task.Output
		err = json.NewDecoder(response.Body).Decode(&output)
		require.NoError(t, err)
		require.Equal(t, "Hello, world!", output.Output)
	})

	t.Run("Multipart", func(t *testing.T) {
		runner := &mock.Runner{
			OnFinalize: func(ctx context.Context, output *task.Output) error {
				output.Output = "Hello, world!"
				return nil
			},
		}
		handler := task.NewTaskHandler(tsk, runner)

		// Create a multipart request
		data := map[string]any{
			"context": "value",
		}
		context, err := json.Marshal(data)
		require.NoError(t, err)
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		err = writer.WriteField("context", string(context))
		require.NoError(t, err)

		// Add a file attachment
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("This is an attached file."))
		require.NoError(t, err)

		require.NoError(t, writer.Close())
		request, err := http.NewRequest("POST", "", body)
		require.NoError(t, err)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()

		// Handle the request
		handler.Handle(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var output task.Output
		err = json.NewDecoder(response.Body).Decode(&output)
		require.NoError(t, err)
		require.Equal(t, "Hello, world!", output.Output)
	})

	t.Run("ReplyAttachments", func(t *testing.T) {
		runner := &mock.Runner{
			OnFinalize: func(ctx context.Context, output *task.Output) error {
				output.Output = "Here is a generated file."
				output.Attachments = append(output.Attachments, &attachments.Attachment{
					Filename: "test.txt",
					Data:     []byte("Generated file content."),
				})
				return nil
			},
		}
		response := httptest.NewRecorder()
		handler := task.NewTaskHandler(tsk, runner)
		request, err := http.NewRequest("GET", "?context=value", nil)
		require.NoError(t, err)

		// Handle the request
		handler.Handle(response, request)
		require.Equal(t, http.StatusOK, response.Code)

		// Check for attachments in the response
		contentType := response.Header().Get("Content-Type")
		mediaType, params, err := mime.ParseMediaType(contentType)
		require.Equal(t, "multipart/form-data", mediaType)
		require.NoError(t, err)
		reader := multipart.NewReader(response.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)

			// Should either be the file attachment or the context value
			if part.FileName() == "" {
				require.Equal(t, "text/plain", part.Header.Get("Content-Type"))
				data, err := io.ReadAll(part)
				require.NoError(t, err)
				require.Equal(t, "This is an attached file.", string(data))
			} else {
				require.Equal(t, "context", part.FormName())
				data, err := io.ReadAll(part)
				require.NoError(t, err)
				require.Equal(t, "value", string(data))
			}
		}
	})

	t.Run("BadRequest", func(t *testing.T) {
		runner := &mock.Runner{
			OnFinalize: func(ctx context.Context, output *task.Output) error {
				return nil
			},
		}
		handler := task.NewTaskHandler(tsk, runner)
		request, err := http.NewRequest("POST", "", bytes.NewBuffer([]byte("invalid")))
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.Handle(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code)
	})
}
