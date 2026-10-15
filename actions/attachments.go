package actions

import (
	"crypto/sha256"
	"encoding/hex"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/ulid"
)

// Attachment preserves metadata and a hash of available bytes at capture time.
// It retains neither data nor references to the source attachment. A missing
// hash means the bytes were unavailable, not that the attachment was empty.
type Attachment struct {
	ID          ulid.ULID `json:"id,omitempty"`           // Attachment identity at capture time.
	Filename    string    `json:"filename"`               // Original filename.
	ContentType string    `json:"content_type,omitempty"` // Declared media type.
	URL         string    `json:"url,omitempty"`          // Original remote location, if any.
	Size        int       `json:"size"`                   // Available byte count at capture time.
	SHA256      string    `json:"sha256,omitempty"`       // Digest of captured bytes; absent if unavailable.
}

// RecordAttachment captures metadata and hashes available bytes without fetching
// remote content.
func RecordAttachment(attachment *attachments.Attachment) Attachment {
	if attachment == nil {
		return Attachment{}
	}

	recorded := Attachment{
		ID:          attachment.ID,
		Filename:    attachment.Filename,
		ContentType: attachment.ContentType,
		URL:         attachment.URL,
		Size:        len(attachment.Data),
	}

	// Hash only bytes already available; logging must not trigger a download.
	if attachment.Data != nil {
		hash := sha256.Sum256(attachment.Data)
		recorded.SHA256 = hex.EncodeToString(hash[:])
	}

	return recorded
}

// captureAttachments replaces attachment references with metadata.
func captureAttachments(event *Action) {
	if request := event.GenerationRequest; request != nil {
		for _, file := range request.Attachments {
			if file != nil {
				event.Attachments = append(event.Attachments, RecordAttachment(file))
			}
		}
		request.Attachments = nil
	}

	if response := event.GenerationResponse; response != nil {
		for _, file := range response.Attachments {
			if file != nil {
				event.Attachments = append(event.Attachments, RecordAttachment(file))
			}
		}
		response.Attachments = nil
	}

	// TODO: process capability request/response when/if attachment content is available.
}
