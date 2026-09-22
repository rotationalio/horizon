package horizon

import (
	"encoding/json"
	"net/http"
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
		ok     bool
		err    error
		task   *Task
		input  *Input
		output *Output
	)

	// Load the task from the router.
	// TODO: return a better not found error message using go.rtnl.ai/x api response
	if task, ok = h.router.Get(r.URL.Path); !ok {
		http.NotFound(w, r)
		return
	}

	// Load the input from the request.
	// TODO: multipart loading for attachments.
	// TODO: better validation of the request for better error messages.
	// TODO: better error handling using go.rtnl.ai/x api response.
	// TODO: JSON struct tags for decoding the input.
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: better error handling using go.rtnl.ai/x api response.
	if output, err = task.Run(r.Context(), input, h.runner); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the output to the response.
	// TODO: multipart writing for attachments.
	// TODO: content type handling.
	// TODO: JSON struct tags for encoding the output.
	json.NewEncoder(w).Encode(output)

	// Write the response.
	w.WriteHeader(http.StatusOK)
}

type TaskHandler struct {
	task   *Task
	runner Runner
}

// Implements the http.Handler interface.
func (h *TaskHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		input  *Input
		output *Output
	)

	// Load the input from the request.
	// TODO: multipart loading for attachments.
	// TODO: better validation of the request for better error messages.
	// TODO: better error handling using go.rtnl.ai/x api response.
	// TODO: JSON struct tags for decoding the input.
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: better error handling using go.rtnl.ai/x api response.
	if output, err = h.task.Run(r.Context(), input, h.runner); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the output to the response.
	// TODO: multipart writing for attachments.
	// TODO: content type handling.
	// TODO: JSON struct tags for encoding the output.
	json.NewEncoder(w).Encode(output)

	// Write the response.
	w.WriteHeader(http.StatusOK)
}
