package attachments_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/config"
)

// Verifies in-memory attachment data is returned as bytes, text, and a base64
// data URI without a network request.
func TestAttachmentData(t *testing.T) {
	attachment := &attachments.Attachment{
		Filename:    "note.txt",
		ContentType: "text/plain; charset=utf-8",
		Data:        []byte("hello"),
	}

	data, err := attachment.Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)

	text, err := attachment.Text(context.Background())
	require.NoError(t, err)
	require.Equal(t, "hello", text)

	uri, err := attachment.Base64URI(context.Background())
	require.NoError(t, err)
	require.Equal(t, "data:text/plain;base64,aGVsbG8=", uri)
}

// Clone copies attachment metadata and data without sharing mutable byte storage.
func TestAttachmentClone(t *testing.T) {
	// Setup: use a populated attachment whose data can be mutated independently.
	original := &attachments.Attachment{
		Filename:    "report.txt",
		ContentType: "text/plain",
		URL:         "https://example.com/report.txt",
		Data:        []byte("original"),
	}

	// Execute the clone operation, then mutate its byte buffer.
	cloned := original.Clone()
	cloned.Data[0] = 'X'

	// Assert all scalar fields were copied and the source data is unchanged.
	require.NotSame(t, original, cloned)
	require.Equal(t, "report.txt", cloned.Filename)
	require.Equal(t, "text/plain", cloned.ContentType)
	require.Equal(t, original.URL, cloned.URL)
	require.Equal(t, "original", string(original.Data))
	require.Equal(t, "Xriginal", string(cloned.Data))

	// The collection clone delegates to Attachment.Clone and preserves nil entries.
	collection := attachments.Attachments{original, nil}
	clonedCollection := collection.Clone()
	clonedCollection[0].Data[1] = 'Y'
	require.Equal(t, "original", string(original.Data))
	require.Nil(t, clonedCollection[1])
	require.NotSame(t, original, clonedCollection[0])
	clonedCollection[0] = nil
	require.Same(t, original, collection[0])

	// Nil receivers stay nil, while non-nil empty values remain non-nil.
	var absent *attachments.Attachment
	require.Nil(t, absent.Clone())
	empty := (&attachments.Attachment{Data: []byte{}}).Clone()
	require.NotNil(t, empty.Data)
	var noAttachments attachments.Attachments
	require.Nil(t, noAttachments.Clone())
	require.NotNil(t, attachments.Attachments{}.Clone())
}

func TestBase64URIRejectsInvalidContentType(t *testing.T) {
	attachment := &attachments.Attachment{
		Filename:    "note.txt",
		ContentType: "invalid",
		Data:        []byte("hello"),
	}

	_, err := attachment.Base64URI(context.Background())
	require.Error(t, err)
}

// Verifies URL-backed attachments are downloaded through the shared HTTP client.
func TestAttachmentDownload(t *testing.T) {
	setAttachmentConfig(t, 1024, time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("downloaded"))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	attachment := &attachments.Attachment{Filename: "note.txt", URL: server.URL}
	data, err := attachment.Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, []byte("downloaded"), data)
}

// Verifies cancellation is propagated to URL-backed attachment downloads.
func TestAttachmentDownloadPreCanceled(t *testing.T) {
	setAttachmentConfig(t, 1024, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	attachment := &attachments.Attachment{Filename: "note.txt", URL: "https://example.com"}
	_, err := attachment.Bytes(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

// Infers a data URI type from a filename or bytes and rejects unknown binary data.
func TestBase64URIInfersContentType(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		data       []byte
		wantPrefix string
		wantError  bool
	}{
		{name: "filename extension", filename: "note.txt", data: []byte("hello"), wantPrefix: "data:text/plain;base64,"},
		{name: "content sniffing", filename: "upload", data: []byte("\x89PNG\r\n\x1a\nrest"), wantPrefix: "data:image/png;base64,"},
		{name: "unknown binary", filename: "upload.unknown-ext", data: []byte{0, 1, 2, 3}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachment := &attachments.Attachment{Filename: tt.filename, Data: tt.data}
			uri, err := attachment.Base64URI(context.Background())
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantPrefix+base64.StdEncoding.EncodeToString(tt.data), uri)
		})
	}
}

// Applies the configured response-size ceiling to a remote attachment.
func TestAttachmentDownloadUsesConfiguredSizeLimit(t *testing.T) {
	setAttachmentConfig(t, 5, time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "123456")
	}))
	t.Cleanup(server.Close)

	attachment := &attachments.Attachment{Filename: "large.bin", URL: server.URL}
	_, err := attachment.Bytes(t.Context())
	require.ErrorContains(t, err, "exceeds maximum size of 5 bytes")
}

// Confirms the attachment timeout cancels only its derived request context.
// Downloads a body over 64 MiB when the configured limit is raised above the default.
func TestAttachmentDownloadLimitCanExceedDefault(t *testing.T) {
	defaults, err := config.New()
	require.NoError(t, err)
	defaultMaxBytes := defaults.AttachmentMaxDownloadBytes
	bodySize := defaultMaxBytes + 1
	maxBytes := defaultMaxBytes + (1 << 20)
	setAttachmentConfig(t, maxBytes, 10*time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(bodySize, 10))
		chunk := make([]byte, 1<<20)
		for remaining := bodySize; remaining > 0; {
			writeSize := int64(len(chunk))
			if writeSize > remaining {
				writeSize = remaining
			}
			n, err := w.Write(chunk[:int(writeSize)])
			if err != nil {
				return
			}
			remaining -= int64(n)
		}
	}))
	t.Cleanup(server.Close)

	attachment := &attachments.Attachment{Filename: "large.bin", URL: server.URL}
	data, err := attachment.Bytes(t.Context())
	require.NoError(t, err)
	require.Len(t, data, int(bodySize))
}

// Confirms the attachment timeout cancels only its derived request context.
func TestAttachmentDerivedTimeoutDoesNotCancelCaller(t *testing.T) {
	setAttachmentConfig(t, 1024, 40*time.Millisecond)
	started := make(chan struct{})
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(finished)
	}))
	t.Cleanup(server.Close)

	caller := context.Background()
	attachment := &attachments.Attachment{Filename: "slow.bin", URL: server.URL}
	_, err := attachment.Bytes(caller)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	<-started
	<-finished
	require.NoError(t, caller.Err())
}

// Confirms an earlier caller deadline takes precedence over the configured timeout.
func TestAttachmentRespectsEarlierCallerDeadline(t *testing.T) {
	setAttachmentConfig(t, 1024, time.Second)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	caller, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	attachment := &attachments.Attachment{Filename: "slow.bin", URL: server.URL}
	_, err := attachment.Bytes(caller)
	<-started
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, caller.Err(), context.DeadlineExceeded)
}

// Propagates caller cancellation to an active attachment request.
func TestAttachmentPropagatesCallerCancellation(t *testing.T) {
	setAttachmentConfig(t, 1024, time.Second)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	caller, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := (&attachments.Attachment{Filename: "slow.bin", URL: server.URL}).Bytes(caller)
		result <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.ErrorIs(t, caller.Err(), context.Canceled)
}

// Sets isolated attachment download limits for a test and resets global config afterward.
func setAttachmentConfig(t *testing.T, maxBytes int64, timeout time.Duration) {
	t.Helper()
	t.Cleanup(config.Reset)
	conf, err := config.New()
	require.NoError(t, err)
	conf.AttachmentMaxDownloadBytes = maxBytes
	conf.AttachmentDownloadTimeout = timeout
	require.NoError(t, config.Set(*conf))
}
