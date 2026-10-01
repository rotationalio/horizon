package horizon

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/prompts"
)

// Input is the raw input data that gets passed to a task to execute it. The input
// can come from a user and should supply the context information needed to render a
// prompt as well as any input attachments such as image references, files, videos,
// etc. that are returned from the task execution.
type Input struct {
	// The source of the input (can be a user, but can also be previous tasks)
	Source string `json:"source,omitzero"`

	// The unique ID of the previous response to the model. Use this to
	// create multi-turn conversations
	PreviousResponseID string `json:"previous_response_id,omitzero"`

	// A system (or developer) message inserted into the model's context. When used
	// along with `previous_response_id`, the instructions from a previous response
	// will not be carried over to the next response. This makes it simple to swap out
	// system (or developer) messages in new responses.
	Instructions string `json:"instructions,omitzero"`

	// The context of the input that is used to render the prompt template.
	Context prompts.Context `json:"context,omitzero"`

	// Any attachments from a multipart request that are attached to the input.
	Attachments attachments.Attachments `json:"attachments,omitzero"`
}

// Decode values from the URL.
func (i *Input) DecodeValues(values url.Values) (err error) {
	i.Source = values.Get("source")
	i.PreviousResponseID = values.Get("previous_response_id")
	i.Instructions = values.Get("instructions")
	i.Context = prompts.Context{prompts.DefaultContextKey: values.Get("context")}
	return nil
}

// Decode values from a multipart part.
func (i *Input) DecodePart(part *multipart.Part) (err error) {
	// Read the part content
	buf := bytes.NewBuffer(nil)
	if _, err = io.Copy(buf, part); err != nil {
		return err
	}

	// Decode form values.
	if name := part.FormName(); name != "" {
		switch name {
		case "source":
			i.Source = buf.String()
		case "previous_response_id":
			i.PreviousResponseID = buf.String()
		case "instructions":
			i.Instructions = buf.String()
		case "context":
			return i.Context.UnmarshalJSON(buf.Bytes())
		default:
			return fmt.Errorf("unknown form name: %s", name)
		}
	}

	// Decode file attachments.
	if filename := part.FileName(); filename != "" {
		i.Attachments = append(i.Attachments, &attachments.Attachment{
			Filename: filename,
			Data:     buf.Bytes(),
		})
	}

	return nil
}
