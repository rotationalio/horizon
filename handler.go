package horizon

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/x/api"
	"go.rtnl.ai/x/mime"
)

// Handler routes HTTP requests to tasks executed by its Horizon.
type Handler struct {
	horizon *Horizon
	router  task.Router
}

// Implements http.Handler; Gin can adapt it with gin.WrapH.
var _ http.Handler = (*Handler)(nil)

// ServeHTTP decodes input, executes the routed task, and writes its output.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		ok     bool
		err    error
		tsk    *task.Task
		input  *task.Input
		output *task.Output
	)

	// Load the task from the router.
	if tsk, ok = h.router.Get(r.URL.Path); !ok {
		replyJSON(w, api.NotFound, http.StatusNotFound)
		return
	}

	// Load the input from the request.
	// TODO: better validation of the request for better error messages.
	if input, err = decodeInput(r); err != nil {
		replyJSON(w, api.Error(err), http.StatusBadRequest)
		return
	}

	if output, err = h.horizon.Run(r.Context(), input, tsk); err != nil {
		replyJSON(w, api.Error(err), http.StatusInternalServerError)
		return
	}

	// Write the output to the response.
	if err = replyOutput(w, output); err != nil {
		replyJSON(w, api.Error(err), http.StatusInternalServerError)
		return
	}

}

// Decode the task input from the request.
func decodeInput(r *http.Request) (input *task.Input, err error) {
	// Parse the form.
	if err = r.ParseForm(); err != nil {
		return nil, err
	}

	switch {
	case r.Method == http.MethodGet:
		input = &task.Input{}
		if err = input.DecodeValues(r.Form); err != nil {
			return nil, err
		}
	case strings.HasPrefix(r.Header.Get("Content-Type"), mime.MultipartFormData.String()):
		if input, err = decodeMultipart(r); err != nil {
			return nil, err
		}
	default:
		// Parse the JSON body for standard POST requests.
		input = &task.Input{}
		if err = json.NewDecoder(r.Body).Decode(input); err != nil {
			return nil, err
		}
	}

	return input, nil
}

// Decode a multipart form from the request.
func decodeMultipart(r *http.Request) (input *task.Input, err error) {
	var reader *multipart.Reader
	if reader, err = r.MultipartReader(); err != nil {
		return nil, err
	}

	input = &task.Input{}
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
func replyOutput(w http.ResponseWriter, output *task.Output) (err error) {
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
		if _, err = w.Write(body.Bytes()); err != nil {
			return err
		}
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(reply)
}

// Router returns the handler's router so callers can add, replace, or remove routes.
func (h *Handler) Router() *task.Router {
	return &h.router
}
