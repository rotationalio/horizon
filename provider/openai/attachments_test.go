package openai_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider/openai"
)

// Both protocols attach each input file once, on the last user message, while
// retaining earlier text and supporting attachment-only requests.
func TestAttachmentMappingAcrossAPIs(t *testing.T) {
	t.Run("attachment mapping", func(t *testing.T) {
		tests := []struct {
			name         string
			messages     prompts.Prompts
			wantLastUser int
			wantMessages int
		}{
			{
				name: "multiple users",
				messages: prompts.Prompts{
					{Role: prompts.RoleUser, Content: "first"},
					{Role: prompts.RoleAssistant, Content: "reply"},
					nil,
					{Role: prompts.RoleUser, Content: "last"},
				},
				wantLastUser: 2,
				wantMessages: 3,
			},
			{
				name:         "no user",
				messages:     prompts.Prompts{{Role: prompts.RoleSystem, Content: "system"}},
				wantLastUser: 1,
				wantMessages: 2,
			},
			{
				name:         "no messages",
				wantLastUser: 0,
				wantMessages: 1,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Setup: use in-memory text so placement is independent of downloads.
				files := attachments.Attachments{
					{ContentType: "text/plain", Data: []byte("attached text")},
					{ContentType: "text/plain", Data: []byte("second attachment")},
				}
				originalLength := len(tt.messages)

				// Execute both real converters with the same provider-neutral input.
				chat, err := openai.ChatCompletionsMessages(context.Background(), tt.messages, files)
				require.NoError(t, err)
				responses, err := openai.ResponsesMessages(context.Background(), tt.messages, files)
				require.NoError(t, err)

				// Assert each file appears once, on the last user prompt only.
				require.Len(t, chat, tt.wantMessages)
				require.Len(t, responses.OfInputItemList, tt.wantMessages)
				require.Len(t, chat[tt.wantLastUser].OfUser.Content.OfArrayOfContentParts, 3)
				require.Equal(t, "attached text", chat[tt.wantLastUser].OfUser.Content.OfArrayOfContentParts[1].OfText.Text)
				parts := responses.OfInputItemList[tt.wantLastUser].OfMessage.Content.OfInputItemContentList
				require.Len(t, parts, 3)
				require.Equal(t, "attached text", parts[1].OfInputText.Text)
				require.Equal(t, "second attachment", parts[2].OfInputText.Text)
				require.Equal(t, "second attachment", chat[tt.wantLastUser].OfUser.Content.OfArrayOfContentParts[2].OfText.Text)
				if tt.name == "multiple users" {
					require.Len(t, chat[0].OfUser.Content.OfArrayOfContentParts, 1)
					require.Len(t, responses.OfInputItemList[0].OfMessage.Content.OfInputItemContentList, 1)
				} else {
					require.Equal(t, "This message contains only attachments.", parts[0].OfInputText.Text)
				}
				require.Len(t, tt.messages, originalLength, "converter must not change caller history")
			})
		}
	})

	t.Run("no files does not add a user", func(t *testing.T) {
		// Without attachments the adapters must not invent any messages.
		chat, err := openai.ChatCompletionsMessages(context.Background(), nil, nil)
		require.NoError(t, err)
		responses, err := openai.ResponsesMessages(context.Background(), nil, nil)
		require.NoError(t, err)
		require.Empty(t, chat)
		require.Empty(t, responses.OfInputItemList)
	})
}
