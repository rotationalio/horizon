package task

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"go.rtnl.ai/x/api"
	"go.rtnl.ai/x/mime"
)

// Implements the http.Handler interface for service HTTP requests to execute tasks.
// TODO: add configuration for the handler so that the runner isn't standalone.
// TODO: should the handler wrap a horizon.Horizon struct instead? Or should the
// horizon.Horizon struct implement the http.Handler interface?
// TODO: should the handlers be in their own package?
type Handler struct {
	router *Router
	runner Runner
}

// TODO: ensure all responses are Content-Type: application/json.
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	var (
		ok   bool
		task *Task
	)

	// Load the task from the router.
	if task, ok = h.router.Get(r.URL.Path); !ok {
		replyJSON(w, api.NotFound, http.StatusNotFound)
		return
	}

	// Handle the task; this function handles the input decoding and HTTP response
	// writing.
	handleTask(w, r, task, h.runner)
}

type TaskHandler struct {
	task   *Task
	runner Runner
}

// Implements the http.Handler interface.
func (h *TaskHandler) Handle(w http.ResponseWriter, r *http.Request) {
	handleTask(w, r, h.task, h.runner)
}

func handleTask(w http.ResponseWriter, r *http.Request, task *Task, runner Runner) {
	var (
		err    error
		input  *Input
		output *Output
	)

	if input, err = decodeInput(r); err != nil {
		replyJSON(w, api.Error(err), http.StatusBadRequest)
		return
	}

	if output, err = task.Run(r.Context(), input, runner); err != nil {
		replyJSON(w, api.Error(err), http.StatusInternalServerError)
		return
	}

	if err = replyOutput(w, output); err != nil {
		replyJSON(w, api.Error(err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Decode the task input from the request.
func decodeInput(r *http.Request) (input *Input, err error) {
	// Parse the form.
	if err = r.ParseForm(); err != nil {
		return nil, err
	}

	switch {
	case r.Method == http.MethodGet:
		input = &Input{}
		if err = input.DecodeValues(r.Form); err != nil {
			return nil, err
		}
	case strings.HasPrefix(r.Header.Get("Content-Type"), mime.MultipartFormData.String()):
		if input, err = decodeMultipart(r); err != nil {
			return nil, err
		}
	default:
		// Parse the JSON body for standard POST requests.
		input = &Input{}
		if err = json.NewDecoder(r.Body).Decode(input); err != nil {
			return nil, err
		}
	}

	return input, nil
}

// Decode a multipart form from the request.
func decodeMultipart(r *http.Request) (input *Input, err error) {
	var reader *multipart.Reader
	if reader, err = r.MultipartReader(); err != nil {
		return nil, err
	}

	input = &Input{}
	for {
		var part *multipart.Part
		if part, err = reader.NextPart(); err != nil {
			if err == io.EOF {
				return input, nil
			}
			return nil, err
		}

		if err = input.DecodePart(part); err != nil {
			return nil, err
		}
	}
}

// Write the task output to the HTTP response, handling multipart for attachments.
func replyOutput(w http.ResponseWriter, output *Output) (err error) {
	if len(output.Attachments) > 0 {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Write the JSON output as a form field.
		var data []byte
		if data, err = json.Marshal(output); err != nil {
			return err
		}
		if err = writer.WriteField("output", string(data)); err != nil {
			return err
		}

		// Write the attachments.
		for _, attachment := range output.Attachments {
			var part io.Writer
			if part, err = writer.CreateFormFile(attachment.Filename, attachment.Filename); err != nil {
				return err
			}
			if _, err = io.Copy(part, bytes.NewReader(attachment.Data)); err != nil {
				return err
			}
		}

		// Close the writer.
		if err = writer.Close(); err != nil {
			return err
		}

		// Set the content type boundaries.
		w.Header().Set("Content-Type", writer.FormDataContentType())
	} else {
		// If there are no attachments, this is just a JSON response.
		w.Header().Set("Content-Type", "application/json")
		if err = json.NewEncoder(w).Encode(output); err != nil {
			return err
		}
	}

	return nil
}

// Write a JSON response from the reply.
func replyJSON(w http.ResponseWriter, reply api.Reply, status int) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// Create a new task handler for testing.
func NewTaskHandler(task *Task, runner Runner) *TaskHandler {
	return &TaskHandler{
		task:   task,
		runner: runner,
	}
}
