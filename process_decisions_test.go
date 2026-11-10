package horizon_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/prompts/templates"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
)

// Runs a decisions task whose rendered prompts are the given messages.
func runDecisionsTest(t *testing.T, inference *mockprovider.MockProvider, messages prompts.Prompts) (*task.Output, error) {
	t.Helper()

	runner := struct {
		*processTestRunner
		*mock.Renderer
	}{
		&processTestRunner{},
		&mock.Renderer{OnRender: func(templates.Templates, prompts.Context) (prompts.Prompts, error) { return messages, nil }},
	}

	config := testProviderConfig()
	config.ID = inference.ID()
	config.APIType = provider.APITypeDecisions

	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return runner, nil }))
	_, err := h.GetOrCreateProvider(config, inference)
	require.NoError(t, err)

	return h.Run(context.Background(), &task.Input{}, &task.Task{
		Provider: &config,
		Model:    task.Model{Slug: "typesafe/jev-1.14"},
		Input:    &schema.Input{Modality: modality.Text},
		Output:   &schema.Output{Modality: modality.Decisions},
	})
}

// The system prompt's JSON becomes the request's questions and leaves the transcript.
func TestProcessRunDecisionsSystemPromptBecomesQuestions(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		require.NotNil(t, request.Decisions)
		require.Equal(t, provider.QuestionNoul, request.Decisions.Questions["is_bug"].Type)
		require.Equal(t, "Is this a defect?", request.Decisions.Questions["is_bug"].Instructions)
		require.Equal(t, "text", request.Decisions.Fields["name"].Type)
		require.Equal(t, "2026-11-09", request.Decisions.ReferenceDate)

		// Only the user prompt remains as input.
		require.Len(t, request.Input, 1)
		require.Equal(t, prompts.RoleUser, request.Input[0].Role)
		require.Equal(t, "Checkout is blank.", request.Input[0].Content)
		return processTestResponse(), nil
	}

	output, err := runDecisionsTest(t, inference, prompts.Prompts{
		{Role: prompts.RoleSystem, Type: prompts.TypeMessage, Content: `{
			"questions": {"is_bug": {"type": "noul", "instructions": "Is this a defect?"}},
			"fields": {"name": {"type": "text"}},
			"reference_date": "2026-11-09"
		}`},
		{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "Checkout is blank."},
	})
	require.NoError(t, err)
	require.Equal(t, "answer", output.Output)
	require.Equal(t, task.OutcomeSucceeded, output.Outcome)
	require.Equal(t, 1, inference.Calls(t, mockprovider.Generate))
}

// Bad or missing decisions JSON fails before any generation call is made.
func TestProcessRunDecisionsRejectsBadSpec(t *testing.T) {
	user := &prompts.Prompt{Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "text"}

	cases := map[string]prompts.Prompts{
		"NotJSON":        {{Role: prompts.RoleSystem, Type: prompts.TypeMessage, Content: "be helpful"}, user},
		"UnknownKey":     {{Role: prompts.RoleSystem, Type: prompts.TypeMessage, Content: `{"question": {}}`}, user},
		"NoSystemPrompt": {user},
	}
	for name, messages := range cases {
		t.Run(name, func(t *testing.T) {
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				t.Fatal("generate must not be called")
				return nil, nil
			}

			output, err := runDecisionsTest(t, inference, messages)
			require.ErrorIs(t, err, errors.ErrDecisionSpecRequired)
			require.Equal(t, task.OutcomeFailed, output.Outcome)
		})
	}
}
