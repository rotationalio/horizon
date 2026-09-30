package attachments

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	stdhttp "net/http"
	"time"

	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
)

const (
	// Avoids unbounded memory use while preparing provider requests.
	MaximumDownloadSize int64 = 64 << 20

	// Maximum duration of a remote attachment request.
	DownloadTimeout = 16 * time.Second
)

type Attachments []*Attachment

type Attachment struct {
	ID          ulid.ULID `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"` // The ID of the attachment
	Filename    string    `json:"filename" yaml:"filename" msg:"filename"`             // The filename of the attachment
	ContentType string    `json:"content_type" yaml:"content_type" msg:"content_type"` // The content type of the attachment
	URL         string    `json:"url,omitempty" yaml:"url,omitempty" msg:"url,omitempty"`
	Data        []byte    `json:"data" yaml:"data" msg:"data"` // The data of the attachment
}

// Returns the attachment data, downloading it when only a URL is provided.
func (a *Attachment) Bytes() ([]byte, error) {
	return a.BytesContext(context.Background())
}

// Returns the attachment data, downloading it with ctx when only a URL is
// provided. Remote bodies larger than [MaximumDownloadSize] are rejected.
func (a *Attachment) BytesContext(ctx context.Context) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("attachment is nil")
	}
	if len(a.Data) > 0 {
		return a.Data, nil
	}
	if a.URL == "" {
		return nil, fmt.Errorf("attachment %q has no data or URL", a.Filename)
	}

	ctx, cancel := context.WithTimeout(ctx, DownloadTimeout)
	defer cancel()

	// TODO: Once the shared HTTP client supports middleware, reject loopback,
	// private, link-local, and other non-public destinations after DNS
	// resolution, and re-check every redirect to prevent SSRF.
	req, err := http.NewRequestWithContext(ctx, stdhttp.MethodGet, a.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < stdhttp.StatusOK || resp.StatusCode >= stdhttp.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch attachment %q: %s", a.Filename, resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaximumDownloadSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaximumDownloadSize {
		return nil, fmt.Errorf("attachment %q exceeds maximum download size of %d bytes", a.Filename, MaximumDownloadSize)
	}
	return data, nil
}

// Returns the attachment data as text.
func (a *Attachment) Text() (string, error) {
	return a.TextContext(context.Background())
}

// Returns the attachment data as text, using ctx for downloads.
func (a *Attachment) TextContext(ctx context.Context) (string, error) {
	data, err := a.BytesContext(ctx)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Returns the attachment as a base64-encoded data URI.
func (a *Attachment) Base64URI() (string, error) {
	return a.Base64URIContext(context.Background())
}

// Returns the attachment as a base64-encoded data URI, using ctx for downloads.
func (a *Attachment) Base64URIContext(ctx context.Context) (string, error) {
	data, err := a.BytesContext(ctx)
	if err != nil {
		return "", err
	}
	contentType, err := mime.Parse(a.ContentType)
	if err != nil {
		return "", fmt.Errorf("attachment %q has invalid content type %q: %w", a.Filename, a.ContentType, err)
	}
	return "data:" + contentType.Type.String() + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
