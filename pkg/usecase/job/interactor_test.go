package job_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/m-mizutani/hecatoncheires/pkg/agent/interaction"
	"github.com/m-mizutani/hecatoncheires/pkg/agent/runtrace"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/interfaces"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
	"github.com/m-mizutani/hecatoncheires/pkg/i18n"
	"github.com/m-mizutani/hecatoncheires/pkg/repository/memory"
	slacksvc "github.com/m-mizutani/hecatoncheires/pkg/service/slack"
	jobagent "github.com/m-mizutani/hecatoncheires/pkg/usecase/agent/job"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase/job"
	"github.com/m-mizutani/hecatoncheires/pkg/utils/logging"
)

type postedForm struct {
	channelID string
	threadTS  string
	blocks    []goslack.Block
	text      string
}

type updatedForm struct {
	channelID string
	timestamp string
	blocks    []goslack.Block
	text      string
}

type fakeQuestionPoster struct {
	posts      []postedForm
	updates    []updatedForm
	ephemerals []ephemeralPost
	returnTS   string
	returnErr  error
	// ephemeralErr, when set, is returned by every PostEphemeral after the
	// attempt is recorded.
	ephemeralErr error
	// locales maps a Slack user id to the locale GetUserInfo reports.
	locales map[string]string
}

type ephemeralPost struct {
	channelID string
	userID    string
	text      string
}

func (f *fakeQuestionPoster) PostEphemeral(_ context.Context, channelID, userID, text string) error {
	f.ephemerals = append(f.ephemerals, ephemeralPost{channelID: channelID, userID: userID, text: text})
	return f.ephemeralErr
}

func (f *fakeQuestionPoster) GetUserInfo(_ context.Context, userID string) (*slacksvc.User, error) {
	return &slacksvc.User{ID: userID, Locale: f.locales[userID]}, nil
}

// workspaceAccessStub allows every workspace except denied.
type workspaceAccessStub struct {
	denied string
}

func (s workspaceAccessStub) Authorize(_ context.Context, workspaceID, _ string) error {
	if workspaceID == s.denied {
		return goerr.Wrap(model.ErrWorkspaceAccessDenied, "denied by stub")
	}
	return nil
}

func (s workspaceAccessStub) AuthorizeCurrentUser(ctx context.Context, workspaceID string) error {
	return s.Authorize(ctx, workspaceID, "")
}

func (s workspaceAccessStub) FilterAccessible(_ context.Context, entries []*model.WorkspaceEntry, _ string) ([]*model.WorkspaceEntry, error) {
	return entries, nil
}

func (s workspaceAccessStub) FilterAccessibleForCurrentUser(_ context.Context, entries []*model.WorkspaceEntry) ([]*model.WorkspaceEntry, error) {
	return entries, nil
}

func (f *fakeQuestionPoster) PostThreadMessage(_ context.Context, channelID, threadTS string, blocks []goslack.Block, text string, _ ...slacksvc.PostThreadOption) (string, error) {
	f.posts = append(f.posts, postedForm{channelID: channelID, threadTS: threadTS, blocks: blocks, text: text})
	if f.returnErr != nil {
		return "", f.returnErr
	}
	return f.returnTS, nil
}

func (f *fakeQuestionPoster) UpdateMessage(_ context.Context, channelID, timestamp string, blocks []goslack.Block, text string) error {
	f.updates = append(f.updates, updatedForm{channelID: channelID, timestamp: timestamp, blocks: blocks, text: text})
	return nil
}

func newRunningLog(key model.JobRunKey, runID string, started time.Time) *model.JobRunLog {
	return &model.JobRunLog{
		WorkspaceID:  key.WorkspaceID,
		CaseID:       key.CaseID,
		JobID:        key.JobID,
		RunID:        runID,
		TraceID:      "trace-" + runID,
		Stage:        model.JobRunStageRunning,
		StartedAt:    started,
		ExecutorKind: "planexec",
	}
}

// An answer from a user the Job's workspace policy denies is not delivered:
// the run stays suspended and only the answering user is told why.
func TestHandleQuestionSubmit_WorkspaceAccessDenied(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	key := jobKey("denied")
	now := time.Now().UTC()
	suspended := newRunningLog(key, "RUN-1", now)
	suspended.Stage = model.JobRunStageAwaitingInput
	suspended.PendingInteraction = &model.PendingInteraction{
		PostedChannelID: "C-CASE",
		PostedMessageTS: "FORM-TS-1",
		Reason:          "r",
		Items:           []model.PendingInteractionItem{{ID: "env", Text: "Which environment?", Type: "select", Options: []string{"prod", "stg"}}},
	}
	gt.NoError(t, repo.JobRunLog().Create(ctx, suspended)).Required()

	poster := &fakeQuestionPoster{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:              repo,
		Registry:          model.NewWorkspaceRegistry(),
		InteractionPoster: poster,
		WorkspaceAccess:   workspaceAccessStub{denied: key.WorkspaceID},
	})
	refValue, err := job.EncodeJobQuestionRefForTest(key, "RUN-1")
	gt.NoError(t, err).Required()
	callback := &goslack.InteractionCallback{}
	callback.Channel.ID = "C-CASE"
	callback.Message.Timestamp = "FORM-TS-1"
	callback.User.ID = "U0MALLORY"

	gt.NoError(t, runner.HandleQuestionSubmit(ctx, callback, &goslack.BlockAction{Value: refValue})).Required()

	got, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, got.Stage).Equal(model.JobRunStageAwaitingInput)
	gt.Array(t, poster.updates).Length(0)
	gt.Array(t, poster.ephemerals).Length(1).Required()
	gt.Value(t, poster.ephemerals[0].channelID).Equal("C-CASE")
	gt.Value(t, poster.ephemerals[0].userID).Equal("U0MALLORY")
	gt.String(t, poster.ephemerals[0].text).NotEqual("")
}

// freeTextQuestionExecutor asks one select and one free_text question on its
// first turn and counts the resumes it is given.
type freeTextQuestionExecutor struct {
	resumes []([]interaction.Answer)
}

func (e *freeTextQuestionExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	out, err := req.Interactor.Solicit(ctx, interaction.Request{
		Reason: "need details",
		Items: []interaction.Item{
			{ID: "env", Text: "Which environment?", Type: interaction.ItemSelect, Options: []string{"prod", "stg"}},
			{ID: "note", Text: "Anything else? (optional)", Type: interaction.ItemFreeText},
		},
	})
	if err != nil {
		return nil, err
	}
	if !out.Paused {
		return nil, goerr.New("freeTextQuestionExecutor: interactor did not pause")
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusAwaitingInput}, nil
}

func (e *freeTextQuestionExecutor) Resume(_ context.Context, _ jobagent.ExecuteRequest, _ model.PendingInteraction, answers []interaction.Answer) (*jobagent.ExecuteResult, error) {
	e.resumes = append(e.resumes, answers)
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// freeTextRunFixture is an interactive Job run suspended on the question
// freeTextQuestionExecutor asks.
type freeTextRunFixture struct {
	runner   *job.JobRunner
	exec     *freeTextQuestionExecutor
	repo     interfaces.Repository
	key      model.JobRunKey
	refValue string
}

func newFreeTextRunFixture(t *testing.T, poster *fakeQuestionPoster) *freeTextRunFixture {
	t.Helper()
	ctx := context.Background()
	wsID := "ws-unanswered"
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo, c := setupCaseWithSlack(t, wsID, "C-CASE", "1700000000.0001")
	j := &model.Job{
		ID:          "interactive_unanswered",
		Prompt:      "x",
		Strategy:    model.JobStrategyPlanexec,
		Interactive: true,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	gt.NoError(t, j.Validate()).Required()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: wsID, Name: "WS"}, Jobs: []*model.Job{j}})

	exec := &freeTextQuestionExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:              repo,
		Registry:          registry,
		LLMClient:         inertLLM(),
		Executors:         map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategyPlanexec: exec},
		InteractionPoster: poster,
		WorkspaceAccess:   workspaceAccessStub{},
		NewRunID:          func() string { return "RUN-1" },
		NewTraceID:        func() string { return "TRACE-1" },
		Clock:             func() time.Time { return now },
	})
	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     now.Add(-time.Second),
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()
	gt.Array(t, poster.posts).Length(1).Required()
	return &freeTextRunFixture{
		runner:   runner,
		exec:     exec,
		repo:     repo,
		key:      model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID},
		refValue: submitValueFromBlocks(t, poster.posts[0].blocks),
	}
}

func (f *freeTextRunFixture) submit(t *testing.T, ctx context.Context, values map[string]map[string]goslack.BlockAction) {
	t.Helper()
	callback := &goslack.InteractionCallback{BlockActionState: &goslack.BlockActionStates{Values: values}}
	callback.Channel.ID = "C-CASE"
	callback.Message.Timestamp = "FORM-TS-1"
	callback.User.ID = "U-ANSWERER"
	gt.NoError(t, f.runner.HandleQuestionSubmit(ctx, callback, &goslack.BlockAction{Value: f.refValue})).Required()
}

// onlyChoiceAnswered answers the select item and leaves the free_text blank.
func onlyChoiceAnswered() map[string]map[string]goslack.BlockAction {
	return map[string]map[string]goslack.BlockAction{
		"job_question_item:env":  {"job_question_choice": {SelectedOption: goslack.OptionBlockObject{Value: "prod"}}},
		"job_question_item:note": {"job_question_free_text": {Value: ""}},
	}
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

// A submit that leaves a question blank does not resume the run, and every
// such submit — not only the first — tells the submitter which questions are
// blank. The re-rendered form is identical from the second rejection on, so
// without the ephemeral the user would see no response at all.
func TestHandleQuestionSubmit_UnansweredQuestions(t *testing.T) {
	ctx, logs := capturingLogCtx()
	poster := &fakeQuestionPoster{returnTS: "FORM-TS-1"}
	f := newFreeTextRunFixture(t, poster)
	exec, repo, key := f.exec, f.repo, f.key

	f.submit(t, ctx, onlyChoiceAnswered())
	f.submit(t, ctx, onlyChoiceAnswered())

	gt.Array(t, poster.ephemerals).Length(2).Required()
	for _, eph := range poster.ephemerals {
		gt.String(t, eph.channelID).Equal("C-CASE")
		gt.String(t, eph.userID).Equal("U-ANSWERER")
		gt.String(t, eph.text).Contains("Anything else? (optional)")
		gt.String(t, eph.text).NotContains("Which environment?")
	}
	// The form is still re-rendered with the banner on each rejection.
	gt.Array(t, poster.updates).Length(2).Required()
	for _, upd := range poster.updates {
		gt.String(t, upd.timestamp).Equal("FORM-TS-1")
		banner, ok := upd.blocks[0].(*goslack.SectionBlock)
		gt.Bool(t, ok).True().Required()
		gt.String(t, banner.Text.Text).Equal(":warning: Please answer: Anything else? (optional)")
	}
	gt.Array(t, exec.resumes).Length(0)
	suspended, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, suspended.Stage).Equal(model.JobRunStageAwaitingInput)

	// Each rejection is logged at INFO with the form's identity, the blank
	// item ids and the submitter — and nothing the user typed.
	recs := logRecords(t, logs, "job question submit rejected: unanswered items")
	gt.Array(t, recs).Length(2).Required()
	for _, rec := range recs {
		gt.Value(t, rec["level"]).Equal("INFO")
		gt.Value(t, rec["workspace_id"]).Equal(key.WorkspaceID)
		gt.Value(t, rec["case_id"]).Equal(float64(key.CaseID))
		gt.Value(t, rec["job_id"]).Equal(key.JobID)
		gt.Value(t, rec["run_id"]).Equal("RUN-1")
		gt.Value(t, rec["missing_item_ids"]).Equal([]any{"note"})
		gt.Value(t, rec["user_id"]).Equal("U-ANSWERER")
		gt.Number(t, len(rec)).Equal(9) // time, level, msg + the six attributes above
	}

	// Answering every question resumes the run with no further notice.
	f.submit(t, ctx, map[string]map[string]goslack.BlockAction{
		"job_question_item:env":  {"job_question_choice": {SelectedOption: goslack.OptionBlockObject{Value: "prod"}}},
		"job_question_item:note": {"job_question_free_text": {Value: "nothing more"}},
	})

	gt.Array(t, poster.ephemerals).Length(2)
	gt.Array(t, poster.updates).Length(3).Required()
	gt.String(t, poster.updates[2].text).Equal("Answer received.")
	gt.Array(t, exec.resumes).Length(1).Required()
	gt.Array(t, exec.resumes[0]).Length(2).Required()
	gt.String(t, exec.resumes[0][0].Choice).Equal("prod")
	gt.String(t, exec.resumes[0][1].FreeText).Equal("nothing more")
	final, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, final.Stage).Equal(model.JobRunStageSuccess)
	gt.Array(t, logRecords(t, logs, "job question submit rejected: unanswered items")).Length(2)
}

// The notice is written in the submitter's Slack language, not the
// deployment default.
func TestHandleQuestionSubmit_UnansweredNoticeUsesSubmitterLanguage(t *testing.T) {
	poster := &fakeQuestionPoster{returnTS: "FORM-TS-1", locales: map[string]string{"U-ANSWERER": "ja-JP"}}
	f := newFreeTextRunFixture(t, poster)

	f.submit(t, context.Background(), onlyChoiceAnswered())

	gt.Array(t, poster.ephemerals).Length(1).Required()
	jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
	gt.String(t, poster.ephemerals[0].text).Equal(
		i18n.T(jaCtx, i18n.MsgQuestionFormUnanswered, "• Anything else? (optional)"))
}

// A failed notice does not change the outcome of the rejection: the handler
// still succeeds, the form is still re-rendered, and the run stays suspended.
func TestHandleQuestionSubmit_UnansweredNoticeFailureIsNotFatal(t *testing.T) {
	ctx, logs := capturingLogCtx()
	poster := &fakeQuestionPoster{returnTS: "FORM-TS-1", ephemeralErr: goerr.New("slack is down")}
	f := newFreeTextRunFixture(t, poster)

	f.submit(t, ctx, onlyChoiceAnswered())

	gt.Array(t, poster.ephemerals).Length(1)
	gt.Array(t, poster.updates).Length(1)
	gt.Array(t, f.exec.resumes).Length(0)
	got, err := f.repo.JobRunLog().Get(ctx, f.key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, got.Stage).Equal(model.JobRunStageAwaitingInput)
	recs := logRecords(t, logs, "failed to post unanswered job question notice")
	gt.Array(t, recs).Length(1).Required()
	gt.Value(t, recs[0]["level"]).Equal("ERROR")
}

// A free_text item's input is required in the form, so Slack does not label
// it "(optional)" while the submit handler rejects it blank. A select item's
// input and its "Other" fallback stay optional, because filling either one
// answers the item.
func TestJobInteractor_FormInputOptionality(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo := memory.New()
	key := jobKey("optional")
	gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, "RUN-OPT", now))).Required()
	poster := &fakeQuestionPoster{returnTS: "1.2"}
	it := job.NewJobInteractorForTest(repo, poster, key, "RUN-OPT", "C1", "1.1", "U7",
		newRunningLog(key, "RUN-OPT", now), nil, func() time.Time { return now })
	_, err := it.Solicit(ctx, sampleRequest())
	gt.NoError(t, err).Required()
	gt.Array(t, poster.posts).Length(1).Required()

	optional := map[string]bool{}
	for _, b := range poster.posts[0].blocks {
		if in, ok := b.(*goslack.InputBlock); ok {
			optional[in.BlockID] = in.Optional
		}
	}
	gt.Value(t, optional).Equal(map[string]bool{
		"job_question_item:env":       true,
		"job_question_item:env:other": true,
		"job_question_item:note":      false,
	})
}

func TestHandleQuestionSubmit_RequiresWorkspaceAccess(t *testing.T) {
	key := jobKey("unwired")
	refValue, err := job.EncodeJobQuestionRefForTest(key, "RUN-1")
	gt.NoError(t, err).Required()
	runner := job.NewJobRunner(job.RunnerDeps{Repo: memory.New(), Registry: model.NewWorkspaceRegistry()})

	gt.Error(t, runner.HandleQuestionSubmit(context.Background(), &goslack.InteractionCallback{}, &goslack.BlockAction{Value: refValue}))
}

func jobKey(suffix string) model.JobRunKey {
	return model.JobRunKey{
		WorkspaceID: "ws-" + suffix,
		CaseID:      time.Now().UnixNano(),
		JobID:       "job-" + suffix,
	}
}

func sampleRequest() interaction.Request {
	return interaction.Request{
		Reason: "which environment is affected?",
		Items: []interaction.Item{
			{ID: "env", Text: "Which environment?", Type: interaction.ItemSelect, Options: []string{"prod", "stg"}},
			{ID: "note", Text: "Anything else?", Type: interaction.ItemFreeText},
		},
	}
}

func TestJobInteractor_Solicit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)

	t.Run("suspends run and posts form", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("solicit")
		runID := "run-solicit"
		gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()

		poster := &fakeQuestionPoster{returnTS: "1700000000.000999"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "C42", "1699999999.000001", "U7",
			newRunningLog(key, runID, now), nil, func() time.Time { return now })

		out, err := it.Solicit(ctx, sampleRequest())
		gt.NoError(t, err).Required()
		gt.Bool(t, out.Paused).True()

		// The form was posted to the case thread.
		gt.Array(t, poster.posts).Length(1).Required()
		gt.String(t, poster.posts[0].channelID).Equal("C42")
		gt.String(t, poster.posts[0].threadTS).Equal("1699999999.000001")

		// The run log is now AWAITING_INPUT with the question + posted coords.
		log, err := repo.JobRunLog().Get(ctx, key, runID)
		gt.NoError(t, err).Required()
		gt.Value(t, log.Stage).Equal(model.JobRunStageAwaitingInput)
		gt.Value(t, log.PendingInteraction).NotNil().Required()
		gt.String(t, log.PendingInteraction.PostedChannelID).Equal("C42")
		gt.String(t, log.PendingInteraction.PostedMessageTS).Equal("1700000000.000999")
		gt.String(t, log.PendingInteraction.Reason).Equal("which environment is affected?")
		gt.Array(t, log.PendingInteraction.Items).Length(2).Required()
		gt.String(t, log.PendingInteraction.Items[0].ID).Equal("env")
		gt.String(t, log.PendingInteraction.Items[0].Type).Equal("select")
		gt.Array(t, log.PendingInteraction.Items[0].Options).Equal([]string{"prod", "stg"})

		// The JobRun is suspended (marker set, lease released).
		run, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, run.SuspendedRunID).Equal(runID)
		gt.Bool(t, run.IsSuspended()).True()
		gt.Bool(t, run.SuspendedAt.Equal(now)).True()
		gt.Bool(t, run.LeaseUntil.IsZero()).True()
	})

	// The tokens the pre-question turn burned must be persisted when the run
	// pauses; otherwise the resumed turn (which builds a fresh handler and adds
	// to the stored totals) reports only its own half of the run.
	t.Run("persists this turn's token usage on the suspended log", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("tokens")
		runID := "run-tokens"
		runningLog := newRunningLog(key, runID, now)
		gt.NoError(t, repo.JobRunLog().Create(ctx, runningLog)).Required()

		handler := runtrace.NewHandler(repo.JobRunEvent(), runtrace.Routing{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       runID,
			TraceID:     runningLog.TraceID,
		}, func() time.Time { return now })
		handler.EndLLMCall(handler.StartLLMCall(ctx), &trace.LLMCallData{
			Model:        "m",
			InputTokens:  500,
			OutputTokens: 70,
			Response:     &trace.LLMResponse{Texts: []string{"I need to ask"}},
		}, nil)

		poster := &fakeQuestionPoster{returnTS: "1700000000.000123"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "C42", "1699999999.000001", "U7",
			runningLog, handler, func() time.Time { return now })

		_, err := it.Solicit(ctx, sampleRequest())
		gt.NoError(t, err).Required()

		log, err := repo.JobRunLog().Get(ctx, key, runID)
		gt.NoError(t, err).Required()
		gt.Value(t, log.Stage).Equal(model.JobRunStageAwaitingInput)
		gt.Number(t, log.InputTokens).Equal(500)
		gt.Number(t, log.OutputTokens).Equal(70)
		gt.Number(t, log.LLMCallCount).Equal(1)

		// The in-memory RUNNING log stays at zero: the resumed turn re-reads the
		// log from storage, so stamping the shared pointer here would make
		// finishRun count this turn twice.
		gt.Number(t, runningLog.InputTokens).Equal(0)
		gt.Number(t, runningLog.OutputTokens).Equal(0)
		gt.Number(t, runningLog.LLMCallCount).Equal(0)
	})

	t.Run("no slack thread is a hard error", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("nothread")
		runID := "run-nothread"
		gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()

		poster := &fakeQuestionPoster{returnTS: "x"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "", "", "U7",
			newRunningLog(key, runID, now), nil, func() time.Time { return now })

		_, err := it.Solicit(ctx, sampleRequest())
		gt.Error(t, err)
		gt.Array(t, poster.posts).Length(0)
	})

	t.Run("invalid request is rejected", func(t *testing.T) {
		repo := memory.New()
		key := jobKey("badreq")
		runID := "run-badreq"
		gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()

		poster := &fakeQuestionPoster{returnTS: "x"}
		it := job.NewJobInteractorForTest(repo, poster, key, runID, "C1", "1.1", "U7",
			newRunningLog(key, runID, now), nil, func() time.Time { return now })

		_, err := it.Solicit(ctx, interaction.Request{Items: nil})
		gt.Error(t, err)
		gt.Array(t, poster.posts).Length(0)
	})
}

func TestJobInteractor_RoundTripAnswers(t *testing.T) {
	// Build a form, simulate the user's block-action submission state, parse
	// it back, and assert the answers match what was selected/typed.
	pending := &model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1.1",
		Reason:          "r",
		Items: []model.PendingInteractionItem{
			{ID: "env", Text: "Which environment?", Type: "select", Options: []string{"prod", "stg"}},
			{ID: "tags", Text: "Tags?", Type: "multi_select", Options: []string{"a", "b", "c"}},
			{ID: "note", Text: "Notes?", Type: "free_text"},
		},
	}

	state := &goslack.BlockActionStates{
		Values: map[string]map[string]goslack.BlockAction{
			"job_question_item:env": {
				"job_question_choice": {SelectedOption: goslack.OptionBlockObject{Value: "prod"}},
			},
			"job_question_item:tags": {
				"job_question_choice": {SelectedOptions: []goslack.OptionBlockObject{{Value: "a"}, {Value: "c"}}},
			},
			"job_question_item:note": {
				"job_question_free_text": {Value: "rollback already started"},
			},
		},
	}

	answers := job.ParseJobQuestionAnswersForTest(pending, state)
	gt.Array(t, answers).Length(3).Required()

	byID := map[string]interaction.Answer{}
	for _, a := range answers {
		byID[a.ID] = a
	}
	gt.String(t, byID["env"].Choice).Equal("prod")
	gt.Array(t, byID["tags"].Choices).Equal([]string{"a", "c"})
	gt.String(t, byID["note"].FreeText).Equal("rollback already started")
}

func TestJobInteractor_ParseSkipsUnanswered(t *testing.T) {
	pending := &model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1.1",
		Items: []model.PendingInteractionItem{
			{ID: "a", Text: "A?", Type: "free_text"},
			{ID: "b", Text: "B?", Type: "free_text"},
		},
	}
	state := &goslack.BlockActionStates{
		Values: map[string]map[string]goslack.BlockAction{
			"job_question_item:a": {"job_question_free_text": {Value: "answered"}},
			"job_question_item:b": {"job_question_free_text": {Value: "   "}}, // whitespace only → unanswered
		},
	}
	answers := job.ParseJobQuestionAnswersForTest(pending, state)
	gt.Array(t, answers).Length(1).Required()
	gt.String(t, answers[0].ID).Equal("a")
}

func TestJobQuestionRef_RoundTrip(t *testing.T) {
	// Encode happens inside Solicit; here we drive the decode path against a
	// posted form's button value to prove the resume context round-trips.
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo := memory.New()
	key := jobKey("ref")
	runID := "run-ref-123"
	gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, runID, now))).Required()
	poster := &fakeQuestionPoster{returnTS: "1.2"}
	it := job.NewJobInteractorForTest(repo, poster, key, runID, "C1", "1.1", "U7",
		newRunningLog(key, runID, now), nil, func() time.Time { return now })
	_, err := it.Solicit(ctx, sampleRequest())
	gt.NoError(t, err).Required()

	// Pull the Submit button value out of the posted blocks.
	var refValue string
	for _, b := range poster.posts[0].blocks {
		if ab, ok := b.(*goslack.ActionBlock); ok {
			for _, el := range ab.Elements.ElementSet {
				if btn, ok := el.(*goslack.ButtonBlockElement); ok && btn.ActionID == job.ActionIDJobQuestionSubmit {
					refValue = btn.Value
				}
			}
		}
	}
	gt.String(t, refValue).NotEqual("")

	ws, caseID, jobID, gotRunID, err := job.DecodeJobQuestionRefForTest(refValue)
	gt.NoError(t, err).Required()
	gt.String(t, ws).Equal(key.WorkspaceID)
	gt.Number(t, caseID).Equal(key.CaseID)
	gt.String(t, jobID).Equal(key.JobID)
	gt.String(t, gotRunID).Equal(runID)
}
