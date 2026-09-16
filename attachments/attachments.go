package attachments

import "go.rtnl.ai/ulid"

type Attachments []*Attachment

type Attachment struct {
	ID          ulid.ULID `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"` // The ID of the attachment
	Filename    string    `json:"filename" yaml:"filename" msg:"filename"`             // The filename of the attachment
	ContentType string    `json:"content_type" yaml:"content_type" msg:"content_type"` // The content type of the attachment
	Data        []byte    `json:"data" yaml:"data" msg:"data"`                         // The data of the attachment
}
