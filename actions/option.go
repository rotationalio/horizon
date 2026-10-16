package actions

import "go.rtnl.ai/horizon/attachments"

// Option customizes an event before it is recorded. Add still owns the snapshot.
type Option func(*Action)

// WithDetail attaches a human-readable explanation to an event.
func WithDetail(detail string) Option { return func(a *Action) { a.Detail = detail } }

// WithCorrelation links a message or other event to a generation or tool call.
func WithCorrelation(correlation string) Option {
	return func(a *Action) { a.CorrelationID = correlation }
}

// WithAttachments captures metadata/hashes immediately, without retaining files.
// The option can be reused after its source attachments have changed or expired.
func WithAttachments(files ...*attachments.Attachment) Option {
	recorded := make([]Attachment, 0, len(files))
	for _, file := range files {
		if file != nil {
			recorded = append(recorded, RecordAttachment(file))
		}
	}

	return func(a *Action) {
		a.Attachments = append(a.Attachments, recorded...)
	}
}

func applyOptions(event *Action, options []Option) {
	for _, option := range options {
		if option != nil {
			option(event)
		}
	}
}
