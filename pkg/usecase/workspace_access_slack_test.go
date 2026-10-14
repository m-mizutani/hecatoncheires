package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
	"github.com/m-mizutani/hecatoncheires/pkg/i18n"
	"github.com/m-mizutani/hecatoncheires/pkg/repository/memory"
	"github.com/m-mizutani/hecatoncheires/pkg/service/slack"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase"
)

func TestSlackUseCases_NotifyWorkspaceAccessDenied(t *testing.T) {
	i18n.Init(i18n.LangEN)
	ctx := context.Background()

	t.Run("tells the denied user alone, in the permission message", func(t *testing.T) {
		slackMock := &mockSlackService{}
		uc := usecase.NewSlackUseCases(memory.New(), model.NewWorkspaceRegistry(), nil, nil, slackMock)

		uc.NotifyWorkspaceAccessDenied(ctx, "ws-any", "C-CASE", "U-DENIED", model.ErrWorkspaceAccessDenied)

		gt.Value(t, slackMock.ephemeralChannelID).Equal("C-CASE")
		gt.Value(t, slackMock.ephemeralUserID).Equal("U-DENIED")
		gt.String(t, slackMock.ephemeralText).Contains(i18n.T(ctx, i18n.MsgUIErrWorkspaceAccessDeniedWhat))
	})

	t.Run("posts nothing without a channel to post in", func(t *testing.T) {
		slackMock := &mockSlackService{}
		uc := usecase.NewSlackUseCases(memory.New(), model.NewWorkspaceRegistry(), nil, nil, slackMock)

		uc.NotifyWorkspaceAccessDenied(ctx, "ws-any", "", "U-DENIED", model.ErrWorkspaceAccessDenied)

		gt.Value(t, slackMock.ephemeralUserID).Equal("")
		gt.Value(t, slackMock.ephemeralText).Equal("")
	})

	t.Run("follows the workspace language over the user's locale", func(t *testing.T) {
		enCtx := i18n.ContextWithLang(ctx, i18n.LangEN)
		jaCtx := i18n.ContextWithLang(ctx, i18n.LangJA)
		for _, tc := range []struct {
			name     string
			wsLang   i18n.Lang
			expected string
		}{
			{name: "workspace ja", wsLang: i18n.LangJA, expected: i18n.T(jaCtx, i18n.MsgUIErrWorkspaceAccessDeniedWhat)},
			{name: "workspace unset", wsLang: "", expected: i18n.T(enCtx, i18n.MsgUIErrWorkspaceAccessDeniedWhat)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				slackMock := &mockSlackService{
					getUserInfoFn: func(_ context.Context, userID string) (*slack.User, error) {
						return &slack.User{ID: userID, Locale: "en-US"}, nil
					},
				}
				registry := model.NewWorkspaceRegistry()
				registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-lang"}, Language: tc.wsLang})
				uc := usecase.NewSlackUseCases(memory.New(), registry, nil, nil, slackMock)

				uc.NotifyWorkspaceAccessDenied(ctx, "ws-lang", "C-CASE", "U-DENIED", model.ErrWorkspaceAccessDenied)

				gt.String(t, slackMock.ephemeralText).Contains(tc.expected)
			})
		}
	})

	t.Run("does nothing when Slack is not wired", func(t *testing.T) {
		uc := usecase.NewSlackUseCases(memory.New(), model.NewWorkspaceRegistry(), nil, nil, nil)

		// Must not panic on the nil poster.
		uc.NotifyWorkspaceAccessDenied(ctx, "ws-any", "C-CASE", "U-DENIED", model.ErrWorkspaceAccessDenied)
	})
}
