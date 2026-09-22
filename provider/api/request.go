package api

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/media"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
)

// Request is meant to generalize all task parameters that are being sent in a single
// call to the LLM. The request should have any and all fields that any api request
// the runner might make can use. Not all request fields will be used by all runners.
// Message is the shared rendered prompt/message contract.
type Message = *prompts.Prompt

// Request is the provider request contract.
type Request struct {
	Model        string                        // The name of the model that the backend will use directly.
	Params       *params.Params                // Parameters to modify model behavior and set on outgoing API requests
	Input        prompts.Prompts               // The input text messages to send to the LLM
	Attachments  []*Attachment                 // Any files, images, links, etc. that are attached to the request
	OutputSchema *Schema                       // The schema of the output to return from the LLM
	Tools        []capabilities.ToolDefinition // Capability tools advertised to the model.
}

// An attachment is a file, image, link, etc. that is attached to a request.
type Attachment struct {
	MimeType media.Type `json:"media_type"` // The MIME type of the attachment.
	Filename string     `json:"filename"`   // The filename of the attachment.
	URL      string     `json:"url"`        // The URI of the attachment, either a remote URL or a base64 encoded data URL.
}

// Download the attachment data from the URL and return the string.
// TODO: Handle media grants for attachments served via the media serve.
func (a *Attachment) Text() (_ string, err error) {
	resp, err := http.Get(a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download attachment: %s", resp.Status)
	}

	var body []byte
	if body, err = io.ReadAll(resp.Body); err != nil {
		return "", err
	}

	return string(body), nil
}

// Return a base64 encoded URI for the attachment data for text attachments.
func (a *Attachment) Base64URI() (string, error) {
	text, err := a.Text()
	if err != nil {
		return "", err
	}
	return "data:" + a.MimeType.String() + ";base64," + base64.StdEncoding.EncodeToString([]byte(text)), nil
}
