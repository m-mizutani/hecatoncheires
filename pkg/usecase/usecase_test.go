package usecase_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestUseCases_ResolveTextDisplays(t *testing.T) {
	ctx := context.Background()

	t.Run("with Slack the channel name and link are returned", func(t *testing.T) {
		var lookedUp [][]string
		slackSvc := &mockSlackService{
			getChannelNamesFn: func(_ context.Context, ids []string) (map[string]string, error) {
				lookedUp = append(lookedUp, ids)
				return map[string]string{"C0123ABCD": "general"}, nil
			},
		}
		uc := usecase.New(memory.New(), model.NewWorkspaceRegistry(),
			usecase.WithLLMClient(&mock.LLMClientMock{}),
			usecase.WithSlackService(slackSvc),
		)

		got, err := uc.ResolveTextDisplays(ctx, types.SemanticSlackChannelID, []string{"C0123ABCD"})
		gt.NoError(t, err).Required()
		gt.Equal(t, got, map[string]definition.Display{
			"C0123ABCD": {Label: "#general", URL: "https://slack.com/archives/C0123ABCD"},
		})
		gt.Equal(t, lookedUp, [][]string{{"C0123ABCD"}})
	})

	t.Run("without Slack the value is reported as unresolved", func(t *testing.T) {
		uc := usecase.New(memory.New(), model.NewWorkspaceRegistry())

		got, err := uc.ResolveTextDisplays(ctx, types.SemanticSlackChannelID, []string{"C0123ABCD"})
		gt.NoError(t, err).Required()
		gt.Equal(t, got, map[string]definition.Display{"C0123ABCD": {}})
	})
}
