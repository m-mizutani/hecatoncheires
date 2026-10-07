package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/m-mizutani/hecatoncheires/pkg/domain/interfaces"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
	"github.com/m-mizutani/hecatoncheires/pkg/i18n"
	"github.com/m-mizutani/hecatoncheires/pkg/repository/agentarchive"
	"github.com/m-mizutani/hecatoncheires/pkg/repository/memory"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase"
	"github.com/m-mizutani/hecatoncheires/pkg/utils/async"
)

// An answer from a user the workspace's policy denies is not processed: the
// form is left as it is, no turn resumes, and the user alone is told why.
func TestHandleThreadCaseQuestionSubmit_WorkspaceAccessDenied(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
	llm := newScriptedClient(nil)
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:            repo,
		Registry:        reg,
		LLM:             llm,
		HistoryRepo:     agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:       agentarchive.NewMemoryTraceRepository(),
		SlackService:    slackMock,
		CaseUC:          caseUC,
		WorkspaceAccess: denyingAccess(t, reg, "support"),
	})
	startAgentRuntime(t, agentRuntimeDeps{UC: agentUC, Repo: repo, Registry: reg, LLM: llm})

	const channel = "C-MONITOR"
	const rootTS = "1700000000.000800"
	cb := &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U-DENIED"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channel}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: "1700000000.000900", ThreadTimestamp: rootTS}},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDThreadCreateQuestionSubmit, Value: channel + ":" + rootTS}},
		},
	}
	gt.NoError(t, agentUC.HandleThreadCaseQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	gt.Array(t, slackMock.updates()).Length(0)
	eph := slackMock.ephemerals()
	gt.Array(t, eph).Length(1).Required()
	gt.Value(t, eph[0].ChannelID).Equal(channel)
	gt.Value(t, eph[0].UserID).Equal("U-DENIED")
}

// A submit that leaves a question blank does not resume the create agent, and
// every such submit — not only the first — tells the submitter which questions
// are blank. The re-rendered form is identical from the second rejection on,
// so without the ephemeral the user would see no response at all.
func TestHandleThreadCaseQuestionSubmit_UnansweredQuestions(t *testing.T) {
	const channel = "C-MONITOR"
	const rootTS = "1700000000.000800"
	const formTS = "1700000000.000900"
	setup := func(t *testing.T, ephemeralErr error) (*usecase.AgentUseCase, *agentTestSlackService, interfaces.Repository) {
		t.Helper()
		ctx := context.Background()
		repo := memory.New()
		reg := newThreadWorkspaceRegistry()
		slackMock := &agentTestSlackService{postEphemeralErr: ephemeralErr}
		caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
		llm := newScriptedClient(nil)
		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     reg,
			LLM:          llm,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
			CaseUC:       caseUC,
		})
		startAgentRuntime(t, agentRuntimeDeps{UC: agentUC, Repo: repo, Registry: reg, LLM: llm})

		now := time.Now().UTC()
		gt.NoError(t, repo.Session().Put(ctx, &model.Session{
			ID:            "ssn-thread-unanswered",
			ChannelID:     channel,
			ThreadTS:      rootTS,
			Kind:          model.SessionKindCase,
			CreatorUserID: "U-REPORTER",
			LastAction:    model.SessionEndedWithQuestion,
			PendingQuestion: &model.PendingQuestion{
				PostedChannelID: channel,
				PostedMessageTS: formTS,
				Reason:          "need details",
				Items: []model.PendingQuestionItem{
					{ID: "q-sev", Text: "Severity?", Type: "select", Options: []string{"high", "low"}},
					{ID: "q-note", Text: "Anything else? (optional)", Type: "free_text"},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		})).Required()
		return agentUC, slackMock, repo
	}
	submit := func(t *testing.T, ctx context.Context, agentUC *usecase.AgentUseCase) {
		t.Helper()
		cb := &goslack.InteractionCallback{
			Type:    goslack.InteractionTypeBlockActions,
			User:    goslack.User{ID: "U-ANSWERER"},
			Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channel}}},
			Message: goslack.Message{Msg: goslack.Msg{Timestamp: formTS, ThreadTimestamp: rootTS}},
			BlockActionState: &goslack.BlockActionStates{
				Values: map[string]map[string]goslack.BlockAction{
					usecase.BlockIDDraftQuestionItemPrefix + "q-sev": {
						usecase.ActionIDDraftQuestionChoice: {SelectedOption: goslack.OptionBlockObject{Value: "high"}},
					},
					usecase.BlockIDDraftQuestionItemPrefix + "q-note": {
						usecase.ActionIDDraftQuestionFreeText: {Value: "  "},
					},
				},
			},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDThreadCreateQuestionSubmit, Value: channel + ":" + rootTS}},
			},
		}
		gt.NoError(t, agentUC.HandleThreadCaseQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
		async.Wait()
	}
	// Not resumed: the question is still pending, no case was created and the
	// agent posted nothing.
	assertNotResumed := func(t *testing.T, repo interfaces.Repository, slackMock *agentTestSlackService) {
		t.Helper()
		ctx := context.Background()
		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn.PendingQuestion).NotNil().Required()
		gt.Value(t, ssn.PendingQuestion.PostedMessageTS).Equal(formTS)
		c, err := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
		gt.Array(t, slackMock.posts()).Length(0)
	}

	t.Run("every rejection notifies the submitter", func(t *testing.T) {
		agentUC, slackMock, repo := setup(t, nil)
		ctx, logs := capturingLogCtx()
		submit(t, ctx, agentUC)
		submit(t, ctx, agentUC)

		eph := slackMock.ephemerals()
		gt.Array(t, eph).Length(2).Required()
		for _, e := range eph {
			gt.Value(t, e.ChannelID).Equal(channel)
			gt.Value(t, e.UserID).Equal("U-ANSWERER")
			gt.String(t, e.Text).Contains("Anything else? (optional)")
			gt.String(t, e.Text).NotContains("Severity?")
		}
		// The form is still re-rendered with the error banner on each rejection.
		updates := slackMock.updates()
		gt.Array(t, updates).Length(2).Required()
		for _, u := range updates {
			gt.Value(t, u.Timestamp).Equal(formTS)
			banner, ok := u.Blocks[1].(*goslack.ContextBlock)
			gt.Bool(t, ok).True().Required()
			gt.Value(t, banner.BlockID).Equal("thread_create_question_error")
		}
		assertNotResumed(t, repo, slackMock)

		// Each rejection is logged at INFO with the form's identity, the
		// blank item ids and the submitter — and nothing the user typed.
		recs := logRecords(t, logs, "thread case question submit rejected: unanswered items")
		gt.Array(t, recs).Length(2).Required()
		for _, rec := range recs {
			gt.Value(t, rec["level"]).Equal("INFO")
			gt.Value(t, rec["workspace_id"]).Equal("support")
			gt.Value(t, rec["case_channel_id"]).Equal(channel)
			gt.Value(t, rec["case_thread_ts"]).Equal(rootTS)
			gt.Value(t, rec["missing_item_ids"]).Equal([]any{"q-note"})
			gt.Value(t, rec["user_id"]).Equal("U-ANSWERER")
			gt.Number(t, len(rec)).Equal(8) // time, level, msg + the five attributes above
		}
	})

	t.Run("a failed notice does not change the rejection", func(t *testing.T) {
		agentUC, slackMock, repo := setup(t, errors.New("slack is down"))
		ctx, logs := capturingLogCtx()
		submit(t, ctx, agentUC)

		gt.Array(t, slackMock.ephemerals()).Length(1)
		gt.Array(t, slackMock.updates()).Length(1)
		assertNotResumed(t, repo, slackMock)
		recs := logRecords(t, logs, "failed to post unanswered question notice")
		gt.Array(t, recs).Length(1).Required()
		gt.Value(t, recs[0]["level"]).Equal("ERROR")
	})
}

// A free_text item's input is required in the form, so Slack does not label
// it "(optional)" while the submit handler rejects it blank. A select item's
// input and its "Other" fallback stay optional, because filling either one
// answers the item.
func TestBuildThreadCreateQuestionBlocks_InputOptionality(t *testing.T) {
	items := []model.PendingQuestionItem{
		{ID: "severity", Text: "How severe is it?", Type: "select", Options: []string{"high", "low"}},
		{ID: "tags", Text: "Tags?", Type: "multi_select", Options: []string{"a", "b"}},
		{ID: "note", Text: "Anything else?", Type: "free_text"},
	}
	blocks, _ := usecase.BuildThreadCreateQuestionBlocksForTest(context.Background(), "need details", items, "C-CASE:1700000000.000100", "U123")
	gt.Value(t, inputOptionality(blocks)).Equal(map[string]bool{
		usecase.BlockIDDraftQuestionItemPrefix + "severity":                                           true,
		usecase.BlockIDDraftQuestionItemPrefix + "severity" + usecase.BlockIDDraftQuestionOtherSuffix: true,
		usecase.BlockIDDraftQuestionItemPrefix + "tags":                                               true,
		usecase.BlockIDDraftQuestionItemPrefix + "tags" + usecase.BlockIDDraftQuestionOtherSuffix:     true,
		usecase.BlockIDDraftQuestionItemPrefix + "note":                                               false,
	})
}

// TestBuildThreadCreateQuestionBlocks_Fallback locks the notification
// fallback of the thread-mode question form to the i18n layer: English is
// the historical hardcoded string, and a Japanese-locale context must yield
// the Japanese translation (compared against the same i18n source the
// production code reads, not hardcoded here).
func TestBuildThreadCreateQuestionBlocks_Fallback(t *testing.T) {
	items := []model.PendingQuestionItem{
		{ID: "severity", Text: "How severe is it?", Type: "select", Options: []string{"high", "low"}},
	}

	t.Run("default context yields English fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildThreadCreateQuestionBlocksForTest(context.Background(), "need severity", items, "C-CASE:1700000000.000100", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal("We need a bit more info to create this case.")
	})

	t.Run("Japanese context yields localized fallback", func(t *testing.T) {
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		blocks, fallback := usecase.BuildThreadCreateQuestionBlocksForTest(jaCtx, "need severity", items, "C-CASE:1700000000.000100", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal(i18n.T(jaCtx, i18n.MsgThreadCaseQuestionFallback))
		gt.Value(t, fallback).NotEqual("We need a bit more info to create this case.")
	})
}

func TestCaseThreadValueCodec(t *testing.T) {
	t.Run("round-trips channel and thread ts", func(t *testing.T) {
		v := usecase.EncodeCaseThreadValueForTest("C-MONITOR", "1700000000.000100")
		ch, ts, ok := usecase.ParseCaseThreadValueForTest(v)
		gt.Bool(t, ok).True()
		gt.String(t, ch).Equal("C-MONITOR")
		gt.String(t, ts).Equal("1700000000.000100")
	})

	t.Run("a bare thread ts (no channel) is not parseable", func(t *testing.T) {
		// No colon → the submit handler rejects it as malformed rather than
		// splitting on the ts's dot.
		_, _, ok := usecase.ParseCaseThreadValueForTest("1700000000.000100")
		gt.Bool(t, ok).False()
	})

	t.Run("empty and separator-only values are rejected", func(t *testing.T) {
		for _, v := range []string{"", ":", "C-ONLY:", ":1700000000.0001"} {
			_, _, ok := usecase.ParseCaseThreadValueForTest(v)
			gt.Bool(t, ok).False()
		}
	})
}
