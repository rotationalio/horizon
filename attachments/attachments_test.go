package attachments_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
)

// Verifies in-memory attachment data is returned as bytes, text, and a base64
// data URI without a network request.
func TestAttachmentData(t *testing.T) {
	attachment := &attachments.Attachment{
		Filename:    "note.txt",
		ContentType: "text/plain",
		Data:        []byte("hello"),
	}

	data, err := attachment.Bytes()
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)

	text, err := attachment.Text()
	require.NoError(t, err)
	require.Equal(t, "hello", text)

	uri, err := attachment.Base64URI()
	require.NoError(t, err)
	require.Equal(t, "data:text/plain;base64,aGVsbG8=", uri)
}

// Verifies URL-backed attachments are downloaded through the shared HTTP
// client.
func TestAttachmentDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("downloaded"))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	attachment := &attachments.Attachment{Filename: "note.txt", URL: server.URL}
	data, err := attachment.BytesContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, []byte("downloaded"), data)
}

// Verifies cancellation is propagated to URL-backed attachment downloads.
func TestAttachmentDownloadContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	attachment := &attachments.Attachment{Filename: "note.txt", URL: "https://example.com"}
	_, err := attachment.BytesContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
