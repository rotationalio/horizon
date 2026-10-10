package attachments

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
)

type Attachments []*Attachment

// Clone returns independent copies of the attachments and their data bytes.
// Nil entries and a nil collection are preserved.
func (a Attachments) Clone() Attachments {
	if a == nil {
		return nil
	}

	cloned := make(Attachments, len(a))
	for i, attachment := range a {
		cloned[i] = attachment.Clone()
	}
	return cloned
}

type Attachment struct {
	ID          ulid.ULID `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"` // The ID of the attachment
	Filename    string    `json:"filename" yaml:"filename" msg:"filename"`             // The filename of the attachment
	ContentType string    `json:"content_type" yaml:"content_type" msg:"content_type"` // The content type of the attachment
	URL         string    `json:"url,omitempty" yaml:"url,omitempty" msg:"url,omitempty"`
	Data        []byte    `json:"data" yaml:"data" msg:"data"` // The data of the attachment
}

// Clone returns an independent copy of the attachment, including its data bytes.
// A nil receiver returns nil.
func (a *Attachment) Clone() *Attachment {
	if a == nil {
		return nil
	}

	cloned := *a
	cloned.Data = bytes.Clone(a.Data)
	return &cloned
}

// Bytes downloads and returns the attachment data, downloading it with ctx when
// only a URL is provided. A non-nil Data slice, even if empty, is already
// resolved. Remote bodies larger than the configured attachment limit are
// rejected.
func (a *Attachment) Bytes(ctx context.Context) ([]byte, error) {
	if a.Data != nil {
		// We have already resolved the data so we do not need to redownload it.
		return a.Data, nil
	}

	if a.URL == "" {
		return nil, fmt.Errorf("attachment %q has no data or URL", a.Filename)
	}

	conf, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("load attachment download configuration: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.AttachmentDownloadTimeout)
	defer cancel()

	data, err := http.Get(ctx, a.URL, nil, http.GetOptions{
		Accept:       "*/*",
		MaxBodyBytes: conf.AttachmentMaxDownloadBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch attachment %q: %w", a.Filename, err)
	}
	return data, nil
}

// Text returns the attachment data as text. If the attachment data is not
// already resolved, it will be downloaded using ctx.
func (a *Attachment) Text(ctx context.Context) (string, error) {
	data, err := a.Bytes(ctx)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Base64URI returns the attachment as a base64-encoded data URI. If the
// attachment data is not already resolved, it will be downloaded using ctx.
func (a *Attachment) Base64URI(ctx context.Context) (string, error) {
	data, err := a.Bytes(ctx)
	if err != nil {
		return "", err
	}
	contentType, err := a.contentType(data)
	if err != nil {
		return "", err
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// Resolves the explicit content type or infers one from the filename or bytes.
func (a *Attachment) contentType(data []byte) (string, error) {
	if strings.TrimSpace(a.ContentType) != "" {
		contentType, err := mime.Parse(a.ContentType)
		if err != nil {
			return "", fmt.Errorf("attachment %q has invalid content type %q: %w", a.Filename, a.ContentType, err)
		}
		return contentType.Type.String(), nil
	}

	inferred := mime.TypeByExtension(a.Filename)
	if inferred == mime.UnknownMimeType {
		inferred = mime.DetectType(data)
		if inferred == mime.UnknownMimeType || inferred == "application/octet-stream" {
			return "", fmt.Errorf("attachment %q has no content type and its type could not be inferred", a.Filename)
		}
	}

	contentType, err := mime.Parse(string(inferred))
	if err != nil {
		return "", fmt.Errorf("infer content type for attachment %q: %w", a.Filename, err)
	}
	return contentType.Type.String(), nil
}
