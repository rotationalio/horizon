package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// MarshalJSON encodes the log as an ordered JSON array. An action that cannot
// be encoded is replaced with a TypeError diagnostic at the same sequence and
// correlation, preserving the rest of the log and leaving memory unchanged.
func (a *Actions) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("null"), nil
	}

	// Encoding runs outside the lock against immutable event references.
	entries := a.entrySnapshot()
	if len(entries) == 0 {
		return []byte("[]"), nil
	}

	encoded := make([]any, len(entries))
	for i, event := range entries {
		data, err := json.Marshal(event)
		if err != nil {
			// Replace only this entry. Use fresh metadata so even an invalid
			// original timestamp cannot prevent the diagnostic from encoding.
			encoded[i] = &Action{
				Sequence:      event.Sequence,
				CorrelationID: event.CorrelationID,
				Timestamp:     time.Now().UTC(),
				Type:          TypeError,
				Sender:        EntitySystem,
				Receiver:      EntitySystem,
				Detail:        fmt.Sprintf("could not encode %s action", event.Type),
				Error:         fmt.Sprintf("error encoding action: %v", err),
			}
			continue
		}
		encoded[i] = json.RawMessage(data)
	}

	return json.Marshal(encoded)
}

// UnmarshalJSON replaces the log atomically and restores append counters. Null
// resets it to its zero value. Malformed entries leave the previous log intact.
// Missing sequences are filled; supplied sequences must be strictly increasing.
func (a *Actions) UnmarshalJSON(data []byte) error {
	if a == nil {
		return fmt.Errorf("cannot decode into a nil action log")
	}

	// Decode into temporary state so a malformed replacement cannot erase history.
	var entries []*Action
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	var decoded Actions
	for i, event := range entries {
		if event == nil {
			return fmt.Errorf("action %d is null", i)
		}

		if event.Type <= TypeUnknown || int(event.Type) >= len(typeNames) {
			return fmt.Errorf("action %d has an unknown type", i)
		}

		// The sequence is the event identity. Restore omissions, but reject
		// duplicate or reordered identities before replacing the existing log.
		if event.Sequence == 0 {
			event.Sequence = decoded.sequence + 1
		}

		if event.Sequence <= decoded.sequence {
			return fmt.Errorf("action %d has an out-of-order sequence", i)
		}

		// Decoding already owns the payload. Strip file references and reserve
		// counters without cloning or serializing the decoded data again.
		captureAttachments(event)
		decoded.sequence = event.Sequence
		decoded.observeGeneration(event.CorrelationID)
		decoded.entries = append(decoded.entries, event)
	}

	// Replace state, never the mutex. Readers retain their captured event list.
	a.mu.Lock()
	a.entries = decoded.entries
	a.sequence = decoded.sequence
	a.generation = decoded.generation
	a.mu.Unlock()
	return nil
}

// Encode writes the log as JSON, including the newline used by json.Encoder.
// Payload errors use MarshalJSON's diagnostic fallback; writer errors are returned.
func (a *Actions) Encode(w io.Writer) error { return json.NewEncoder(w).Encode(a) }

// Decode reads one JSON log. The returned log is ready for further recording.
func Decode(r io.Reader) (*Actions, error) {
	var log Actions
	if err := json.NewDecoder(r).Decode(&log); err != nil {
		return nil, err
	}
	return &log, nil
}
