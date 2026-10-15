package actions_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/provider"
)

// Source attachment bytes may change after capture; recorded metadata does not.
func TestAttachmentRecords(t *testing.T) {
	file := &attachments.Attachment{Filename: "original.txt", ContentType: "text/plain", Data: []byte("abc")}
	remote := &attachments.Attachment{Filename: "remote", URL: "https://example.com/file"}
	empty := &attachments.Attachment{Filename: "empty", Data: []byte{}}
	var log actions.Actions

	log.AddGenerationRequest(t.Context(), &provider.Request{Attachments: attachments.Attachments{nil, file, remote, empty}})
	log.AddMessage(t.Context(), actions.EntityModel, actions.EntityRunner, nil, actions.WithAttachments(file, nil))
	log.AddGenerationResponse(t.Context(), "generation-1", &provider.Response{Attachments: attachments.Attachments{file}}, nil)

	// Change the live attachment after all three events have captured it.
	file.Filename = "changed.txt"
	file.Data[0] = 'z'
	entry := log.Entries()[0]
	require.Len(t, entry.Attachments, 3)
	require.Equal(t, "original.txt", entry.Attachments[0].Filename)
	require.Equal(t, 3, entry.Attachments[0].Size)
	require.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", entry.Attachments[0].SHA256)
	require.Empty(t, entry.Attachments[1].SHA256, "unavailable bytes have no hash")
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", entry.Attachments[2].SHA256)
	require.Nil(t, entry.GenerationRequest.Attachments, "attachment metadata is stored once on the event")
	require.Equal(t, entry.Attachments[0], log.Entries()[1].Attachments[0])
	require.Equal(t, entry.Attachments[0], log.Entries()[2].Attachments[0])

	// JSON preserves capture metadata, not source file bytes.
	data, err := json.Marshal(&log)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"data"`)
	require.NotContains(t, string(data), "YWJj")
	require.Contains(t, string(data), entry.Attachments[0].SHA256)
	require.Equal(t, actions.Attachment{}, actions.RecordAttachment(nil))
}

// Once captured, source files may be mutated while the log is read and the same
// attachment option is reused. Neither the event nor the option retains a file.
func TestAttachmentCaptureConcurrentMutation(t *testing.T) {
	file := &attachments.Attachment{Filename: "original.txt", Data: []byte("abc")}
	option := actions.WithAttachments(file)
	log := actions.New()
	log.AddGenerationRequest(t.Context(), &provider.Request{Attachments: attachments.Attachments{file}})
	captured := log.Entries()[0].Attachments[0]
	start := make(chan struct{})
	errors := make(chan error, 1)
	var workers sync.WaitGroup

	workers.Go(func() {
		<-start
		for i := range 100 {
			file.Filename = fmt.Sprintf("changed-%d", i)
			file.Data[0]++
		}
	})
	workers.Go(func() {
		<-start
		for range 30 {
			log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, nil, option)
			log.Entries()
			if _, err := json.Marshal(log); err != nil {
				errors <- err
				return
			}
		}
	})

	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	for _, entry := range log.Entries() {
		require.Equal(t, []actions.Attachment{captured}, entry.Attachments)
	}
}
