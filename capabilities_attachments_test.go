package horizon_test

//=============================================================================
// Tests attachment caching and ownership across capabilities turns, including
// reuse of downloaded data and isolation from guard and processor mutations.
//=============================================================================

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/prompts/templates"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
)

// Uses URL-backed attachments with both non-empty and empty bodies. Each attachment
// should be fetched only once per execution and included with the last user prompt, while
// neither provider conversion nor guard handling changes the host's original attachment.
func TestCapabilitiesAttachmentCache(t *testing.T) {
	for _, body := range []string{"downloaded", ""} {
		for _, users := range []int{0, 2} {
			t.Run(fmt.Sprintf("body=%q/users=%d", body, users), func(t *testing.T) {
				// Setup: a counting HTTP fixture and a two-turn mock provider.
				var downloads atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					downloads.Add(1)
					_, _ = w.Write([]byte(body))
				}))
				defer server.Close()
				file := &attachments.Attachment{URL: server.URL, ContentType: "text/plain", Filename: "input.txt"}
				runner := newCapabilityTestRunner("lookup")
				renderer := &mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) {
					messages := prompts.Prompts{{Role: prompts.RoleSystem, Type: prompts.TypeMessage, Content: "system"}}
					for i := 0; i < users; i++ {
						messages = append(messages, &prompts.Prompt{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: fmt.Sprintf("user %d", i)})
					}
					return messages, nil
				}}
				host := struct {
					*capabilityTestRunner
					*mock.Renderer
				}{runner, renderer}
				turn := 0
				inference := mockprovider.New(ulid.Make())
				inference.OnGenerate = func(ctx context.Context, request *provider.Request) (*provider.Response, error) {
					turn++
					require.Equal(t, int32(1), downloads.Load())
					require.NotNil(t, request.Attachments[0].Data)
					require.Equal(t, body, string(request.Attachments[0].Data))

					// Exercise real adapter conversion on both turns, using the cached body.
					chat, err := openai.ChatCompletionsMessages(ctx, request.Input, request.Attachments)
					require.NoError(t, err)
					responses, err := openai.ResponsesMessages(ctx, request.Input, request.Attachments)
					require.NoError(t, err)
					last := 1
					if users == 2 {
						last = 2
						require.Len(t, chat[1].OfUser.Content.OfArrayOfContentParts, 1)
						require.Len(t, responses.OfInputItemList[1].OfMessage.Content.OfInputItemContentList, 1)
					} else {
						require.Equal(t, "This message contains only attachments.", request.Input[last].Content)
					}
					require.Len(t, chat[last].OfUser.Content.OfArrayOfContentParts, 2)
					require.Len(t, responses.OfInputItemList[last].OfMessage.Content.OfInputItemContentList, 2)
					if turn == 1 {
						return capabilityTestResponse(prompts.ToolCall{CallID: "call", Name: "lookup", Arguments: `{}`}), nil
					}
					return processTestResponse(), nil
				}

				// Execute with application-owned URL-only input.
				output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{}, &task.Input{Attachments: attachments.Attachments{file}})

				// Assert exactly one download, two turns, and an unchanged original attachment.
				require.NoError(t, err)
				require.Equal(t, "answer", output.Output)
				require.Equal(t, int32(1), downloads.Load())
				require.Nil(t, file.Data)
				require.Equal(t, server.URL, file.URL)
				renderer.AssertCalled(t, mock.Render, 1)
				inference.AssertCalled(t, mockprovider.Generate, 2)
			})
		}
	}
}

// Has an attachment processor return an object that remains owned by the application.
// The input guard then mutates its request snapshot on each turn; provider input, cached
// attachment bytes, and the processor's original object should all remain unchanged.
func TestCapabilitiesProcessedAttachmentOwnership(t *testing.T) {
	// Setup: the processor deliberately returns a shared object, not its input clone.
	shared := &attachments.Attachment{Filename: "shared.txt", ContentType: "text/plain", Data: []byte("abc")}
	input := &attachments.Attachment{Filename: "input.txt", Data: []byte("original")}
	runner := newCapabilityTestRunner("lookup")
	processor := &mock.AttachmentProcessor{OnProcessAttachment: func(*attachments.Attachment) (*attachments.Attachment, error) { return shared, nil }}
	host := struct {
		*capabilityTestRunner
		*mock.AttachmentProcessor
	}{runner, processor}
	guardTurns := 0
	runner.InputGuard.OnProtectInput = func(request *provider.Request) error {
		guardTurns++
		require.NotSame(t, shared, request.Attachments[0])
		request.Attachments[0].Data[0] = byte('x' + guardTurns - 1)
		return nil
	}
	turn := 0
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		turn++
		require.Equal(t, "abc", string(request.Attachments[0].Data))
		if turn == 1 {
			return capabilityTestResponse(prompts.ToolCall{CallID: "call", Name: "lookup", Arguments: `{}`}), nil
		}
		return processTestResponse(), nil
	}

	// Execute two turns with one preprocessing pass and two independent guard edits.
	output, err := runProcessTest(t, context.Background(), inference, host, &schema.Output{}, &task.Input{Attachments: attachments.Attachments{input}})

	// Assert guard edits stayed isolated and the attachment-only user was added once.
	require.NoError(t, err)
	require.Equal(t, "abc", string(shared.Data))
	require.Equal(t, "original", string(input.Data))
	require.Len(t, output.Messages, 4)
	require.Equal(t, "This message contains only attachments.", output.Messages[0].Content)
	processor.AssertCalled(t, mock.ProcessAttachment, 1)
	runner.InputGuard.AssertCalled(t, mock.ProtectInput, 2)
}
