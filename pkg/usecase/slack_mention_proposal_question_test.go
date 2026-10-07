package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model/config"
	"github.com/m-mizutani/hecatoncheires/pkg/i18n"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/m-mizutani/hecatoncheires/pkg/utils/async"
	"github.com/m-mizutani/hecatoncheires/pkg/utils/logging"
)

// Answering a draft question starts a new turn, and that turn is offered only
// the workspaces the answering user may access. When the answering user may
// use no workspace — or the decision cannot be made — the answer is not
// consumed: the form is left as it is and the question stays pending, so
// someone the policy allows can still answer it.
func TestHandleQuestionSubmit_NoAccessibleWorkspace(t *testing.T) {
	const formTS = "1700000015.000000"
	setup := func(t *testing.T) *dispatcherFixture {
		t.Helper()
		f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
		gt.NoError(t, f.repo.Session().SetPendingQuestion(context.Background(), f.channelID, f.threadTS,
			&model.PendingQuestion{
				PostedChannelID: f.channelID,
				PostedMessageTS: formTS,
				Reason:          "need severity",
				Items:           []model.PendingQuestionItem{{ID: "q-sev", Text: "What is the severity?", Type: "select", Options: []string{"low", "high"}}},
			})).Required()
		return f
	}
	assertQuestionStillOpen := func(t *testing.T, f *dispatcherFixture) {
		t.Helper()
		f.assertNoTurn(t, formTS)
		gt.Array(t, f.slackMock.updates()).Length(0)
		ssn, err := f.repo.Session().GetByThread(context.Background(), f.channelID, f.threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn.PendingQuestion).NotNil().Required()
		gt.Value(t, ssn.PendingQuestion.PostedMessageTS).Equal(formTS)
	}

	t.Run("every workspace denied", func(t *testing.T) {
		f := setup(t)
		registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
		usecase.SetMentionProposalWorkspaceAccessForTest(f.mentionProposal, denyingAccess(t, registry, "ws-1"))

		cb := newDraftQuestionSubmitCallback(f.channelID, f.threadTS, formTS)
		gt.NoError(t, f.mentionProposal.HandleQuestionSubmit(context.Background(), cb, cb.ActionCallback.BlockActions[0])).Required()
		async.Wait()

		assertQuestionStillOpen(t, f)
		gt.Array(t, f.slackMock.texts()).Length(1).Required()
		gt.String(t, f.slackMock.texts()[0]).Contains("No workspace is available to you")
	})

	t.Run("decision fails", func(t *testing.T) {
		f := setup(t)
		access := newAccessFixture(t, time.Minute, map[string]string{"ws-1": policyAllowAll}, "ws-1")
		access.users.failing.Store(true)
		usecase.SetMentionProposalWorkspaceAccessForTest(f.mentionProposal, access.uc)

		cb := newDraftQuestionSubmitCallback(f.channelID, f.threadTS, formTS)
		err := f.mentionProposal.HandleQuestionSubmit(context.Background(), cb, cb.ActionCallback.BlockActions[0])
		gt.Error(t, err).Is(errUserStoreDown)
		async.Wait()

		assertQuestionStillOpen(t, f)
		gt.Array(t, f.slackMock.texts()).Length(0)
	})
}

// A submit that leaves a question blank is not consumed, and every such submit
// — not only the first — tells the submitter which questions are blank. The
// re-rendered form is identical from the second rejection on, so without the
// ephemeral the user would see no response at all.
func TestHandleQuestionSubmit_UnansweredQuestions(t *testing.T) {
	const formTS = "1700000015.000000"
	setup := func(t *testing.T) *dispatcherFixture {
		t.Helper()
		f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
		gt.NoError(t, f.repo.Session().SetPendingQuestion(context.Background(), f.channelID, f.threadTS,
			&model.PendingQuestion{
				PostedChannelID: f.channelID,
				PostedMessageTS: formTS,
				Reason:          "need details",
				Items: []model.PendingQuestionItem{
					{ID: "q-sev", Text: "What is the severity?", Type: "select", Options: []string{"low", "high"}},
					{ID: "q-note", Text: "Anything else? (optional)", Type: "free_text"},
				},
			})).Required()
		return f
	}
	submit := func(t *testing.T, ctx context.Context, f *dispatcherFixture) {
		t.Helper()
		cb := newDraftQuestionSubmitCallback(f.channelID, f.threadTS, formTS)
		gt.NoError(t, f.mentionProposal.HandleQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
		async.Wait()
	}
	assertStillPending := func(t *testing.T, f *dispatcherFixture) {
		t.Helper()
		f.assertNoTurn(t, formTS)
		ssn, err := f.repo.Session().GetByThread(context.Background(), f.channelID, f.threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn.PendingQuestion).NotNil().Required()
		gt.Value(t, ssn.PendingQuestion.PostedMessageTS).Equal(formTS)
	}

	t.Run("every rejection notifies the submitter", func(t *testing.T) {
		f := setup(t)
		ctx, logs := capturingLogCtx()
		submit(t, ctx, f)
		submit(t, ctx, f)

		eph := f.slackMock.ephemerals()
		gt.Array(t, eph).Length(2).Required()
		for _, e := range eph {
			gt.Value(t, e.ChannelID).Equal(f.channelID)
			gt.Value(t, e.UserID).Equal("U1")
			gt.String(t, e.Text).Contains("Anything else? (optional)")
			gt.String(t, e.Text).NotContains("What is the severity?")
		}
		// The form is still re-rendered with the banner on each rejection.
		updates := f.slackMock.updates()
		gt.Array(t, updates).Length(2).Required()
		for _, u := range updates {
			banner, ok := u.rawBlocks[0].(*goslack.SectionBlock)
			gt.Bool(t, ok).True().Required()
			gt.String(t, banner.Text.Text).Equal(":warning: Please answer: Anything else? (optional)")
		}
		assertStillPending(t, f)

		// Each rejection is logged at INFO with the form's identity, the
		// blank item ids and the submitter — and nothing the user typed.
		ssn, err := f.repo.Session().GetByThread(context.Background(), f.channelID, f.threadTS)
		gt.NoError(t, err).Required()
		recs := logRecords(t, logs, "draft question submit rejected: unanswered items")
		gt.Array(t, recs).Length(2).Required()
		for _, rec := range recs {
			gt.Value(t, rec["level"]).Equal("INFO")
			gt.Value(t, rec["workspace_id"]).Equal("")
			gt.Value(t, rec["proposal_id"]).Equal(string(ssn.ProposalID))
			gt.Value(t, rec["channel_id"]).Equal(f.channelID)
			gt.Value(t, rec["thread_ts"]).Equal(f.threadTS)
			gt.Value(t, rec["missing_item_ids"]).Equal([]any{"q-note"})
			gt.Value(t, rec["user_id"]).Equal("U1")
			gt.Number(t, len(rec)).Equal(9) // time, level, msg + the six attributes above
		}
	})

	t.Run("a failed notice does not change the rejection", func(t *testing.T) {
		f := setup(t)
		f.slackMock.ephemeralErr = errors.New("slack is down")
		ctx, logs := capturingLogCtx()
		submit(t, ctx, f)

		gt.Array(t, f.slackMock.ephemerals()).Length(1)
		gt.Array(t, f.slackMock.updates()).Length(1)
		assertStillPending(t, f)
		recs := logRecords(t, logs, "failed to post unanswered question notice")
		gt.Array(t, recs).Length(1).Required()
		gt.Value(t, recs[0]["level"]).Equal("ERROR")
	})
}

// capturingLogCtx returns a context whose logger writes JSON records to the
// returned buffer.
func capturingLogCtx() (context.Context, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

// logRecords decodes every JSON log record whose msg equals msg.
func logRecords(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		gt.NoError(t, json.Unmarshal([]byte(line), &rec)).Required()
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

// A free_text item's input is required in the form, so Slack does not label
// it "(optional)" while the submit handler rejects it blank. A select item's
// input and its "Other" fallback stay optional, because filling either one
// answers the item.
func TestBuildProposalQuestionBlocks_InputOptionality(t *testing.T) {
	q := proposal.QuestionPayload{
		Reason: "need details",
		Items: []proposal.QuestionItem{
			{ID: "severity", Text: "How severe is it?", Type: proposal.QuestionItemSelect, Options: []string{"high", "low"}},
			{ID: "tags", Text: "Tags?", Type: proposal.QuestionItemMultiSelect, Options: []string{"a", "b"}},
			{ID: "note", Text: "Anything else?", Type: proposal.QuestionItemFreeText},
		},
	}
	blocks, _ := usecase.BuildProposalQuestionBlocksForTest(context.Background(), q, "draft-1", "U123")
	gt.Value(t, inputOptionality(blocks)).Equal(map[string]bool{
		usecase.BlockIDDraftQuestionItemPrefix + "severity":                                           true,
		usecase.BlockIDDraftQuestionItemPrefix + "severity" + usecase.BlockIDDraftQuestionOtherSuffix: true,
		usecase.BlockIDDraftQuestionItemPrefix + "tags":                                               true,
		usecase.BlockIDDraftQuestionItemPrefix + "tags" + usecase.BlockIDDraftQuestionOtherSuffix:     true,
		usecase.BlockIDDraftQuestionItemPrefix + "note":                                               false,
	})
}

// inputOptionality maps each input block's block_id to its Optional flag.
func inputOptionality(blocks []goslack.Block) map[string]bool {
	out := map[string]bool{}
	for _, b := range blocks {
		if in, ok := b.(*goslack.InputBlock); ok {
			out[in.BlockID] = in.Optional
		}
	}
	return out
}

func newDraftQuestionSubmitCallback(channelID, threadTS, formTS string) *goslack.InteractionCallback {
	return &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U1"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channelID}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: formTS, ThreadTimestamp: threadTS}},
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				usecase.BlockIDDraftQuestionItemPrefix + "q-sev": {
					usecase.ActionIDDraftQuestionChoice: {SelectedOption: goslack.OptionBlockObject{Value: "high"}},
				},
			},
		},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDDraftQuestionSubmit}},
		},
	}
}

// TestBuildProposalQuestionBlocks_Fallback locks the notification fallback
// of the mention-draft question form to the i18n layer: the English text is
// the historical hardcoded string, and a Japanese-locale context must yield
// the Japanese translation (pulled from the same i18n source the production
// code reads, not hardcoded here).
func TestBuildProposalQuestionBlocks_Fallback(t *testing.T) {
	q := proposal.QuestionPayload{
		Reason: "need severity",
		Items: []proposal.QuestionItem{
			{ID: "severity", Text: "How severe is it?", Type: proposal.QuestionItemSelect, Options: []string{"high", "low"}},
		},
	}

	t.Run("default context yields English fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildProposalQuestionBlocksForTest(context.Background(), q, "draft-1", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal("We need a bit more info to draft this case.")
	})

	t.Run("Japanese context yields localized fallback", func(t *testing.T) {
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		blocks, fallback := usecase.BuildProposalQuestionBlocksForTest(jaCtx, q, "draft-1", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal(i18n.T(jaCtx, i18n.MsgMentionQuestionFallback))
		gt.Value(t, fallback).NotEqual("We need a bit more info to draft this case.")
	})
}
