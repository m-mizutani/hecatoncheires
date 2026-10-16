// Package planexec hosts the reusable plan-and-execute loop shared by
// the proposal (case-draft) host and the planexec-strategy Job host.
//
// The loop is `plan → executePhase → replan → ... → final` and follows the
// vocabulary established by secmon-lab/warren's `pkg/usecase/chat/bluebell`:
// the planner emits a `Plan` (a list of `TaskPlan` to run in parallel), the
// sub-agents fan out via `executePhase`, and the planner is asked again
// (`replan`) with the observations. Every planner output declares its action in
// `next_action` (tasks / direct / question / finalize); the loop exits when the
// planner takes `finalize` — on any round, round 1 included — after which the
// runtime makes the terminal call for the host-visible output.
package planexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/hecatoncheires/pkg/utils/pricing"
)

// Validation limits applied during parsePlanResult / parseReplanResult.
// Adjusting these requires updating the planner system prompt as well so
// the LLM does not propose plans that always reject.
const (
	minTasksPerPhase   = 1
	maxTasksPerPhase   = 5
	minQuestionItems   = 1
	maxQuestionItems   = 5
	minQuestionOptions = 2
	maxToolsPerTask    = 4
)

// rationaleDescription describes the planner's `message` field to the model.
//
// Two halves, and both matter. It says the field is NOT user-facing because the
// previous wording ("shown to the user") invited the planner to write the reply
// there, while nothing has read the field since Sink.PlanProposed was removed in
// #261 — so a planner following it put its answer somewhere that is discarded. And
// it says the rationale is kept, because it is: the whole planner reply is
// committed to the run's conversation and recorded as the LLM_RESPONSE of that
// transition, so `message` is what an operator reading the run's timeline or trace
// archive has to work from when reconstructing why a turn decided as it did. That
// is the field's purpose, and it is why the planner should write something
// substantive rather than a placeholder.
const rationaleDescription = "1-2 sentence rationale for this decision. NOT shown to the user — never put the user's answer here — but it is kept in this run's record, so state the reason plainly enough that someone reading the run later understands the decision."

// The values of `next_action`, the planner's declaration of which action its
// output takes. See nextActionSchema.
const (
	nextActionTasks    = "tasks"
	nextActionDirect   = "direct"
	nextActionQuestion = "question"
	nextActionFinalize = "finalize"
)

// PlanResult is the parsed shape of the first planner-round JSON output.
// The planner chooses exactly one action, named by NextAction:
//   - Tasks: the parallel investigation phase (the default path). At least
//     one task is required.
//   - Direct: skip investigation entirely and answer the user directly
//     (round-1 fast path). Only valid when the host set
//     RunRequest.AllowDirect.
//   - Finalize: there is nothing to investigate or do, so the turn goes
//     straight to its terminal output. This is how a Job whose instructions say
//     "if nothing changed, do nothing" does nothing: without it round 1 had no
//     way to end, and a planner that had decided to do nothing was rejected
//     until it escaped through a tool call (see nextActionSchema).
//
// parsePlanResult normalises the result: NextAction names the chosen action and
// only that action's payload is left set (Finalize is non-nil for a finalize).
type PlanResult struct {
	// Message is a 1-2 sentence rationale for the decision. No code reads it, and
	// that is not a reason to remove it: the planner reply carrying it is committed
	// to the run's conversation and recorded as that transition's LLM_RESPONSE, so
	// it is what the run timeline and the trace archive preserve of WHY the turn
	// decided as it did. Deleting the field would take that out of the record.
	//
	// It is NOT user-facing. The user sees the progress lines the strategy writes
	// and the terminal output, never this. (It once reached the user through
	// Sink.PlanProposed, removed with the in-process Runner in #261.) Do not wire it
	// into a reply — publishing the planner's reasoning as the answer is exactly
	// what rationaleDescription is worded to prevent.
	//
	// The schema marks it Required (see messageSchema, which explains why), but
	// the parser does not: an output with no `message` is accepted as before.
	Message string `json:"message,omitempty"`
	// NextAction names the action this output takes. The schema marks it
	// Required; the parser infers it from the payloads when it is absent, so an
	// output from a provider that ignored the schema is read as before.
	NextAction string `json:"next_action,omitempty"`
	// Tasks is the parallel investigation phase emitted by the planner.
	// Empty / omitted when Direct is set — the two are mutually exclusive.
	Tasks []TaskPlan `json:"tasks,omitempty"`
	// Direct, when non-nil, signals the planner judged the request trivial
	// enough to answer without any investigation phase. The nil Direct (the
	// common case) means "investigate via Tasks". Mutually exclusive with
	// Tasks; rejected unless RunRequest.AllowDirect is true.
	Direct *DirectPlan `json:"direct,omitempty"`
	// Finalize ends the turn on round 1 with nothing investigated.
	Finalize *FinalizePlan `json:"finalize,omitempty"`
}

// DirectPlan is the round-1 "answer directly" payload. It carries only the
// tools the single direct ReAct agent is permitted to call; everything else
// about the direct path (system prompt, history, loop limit) is supplied by
// the runtime, and the response is always plain text — structured-final
// generation (Run[T]) is not consulted on this path.
type DirectPlan struct {
	// Tools is the subset of RunRequest.KnownToolIDs the direct agent may
	// call. May be empty for a pure conversational reply that needs no tool.
	// Bounded by maxToolsPerTask.
	Tools []string `json:"tools,omitempty"`
}

// ReplanResult is the parsed shape of every subsequent planner round. The
// planner must choose EXACTLY ONE terminal-or-continuation action per round:
//   - Tasks: run another investigation phase.
//   - Question: ask the user (only when the host allows it).
//   - Finalize: declare completion and produce the final output.
//
// The action is named by NextAction. An output that takes no action is rejected
// (parseReplanResult) and folded back into another replan round. This is
// deliberate: the previous design treated "empty tasks + no question" as an
// implicit completion signal, so a planner that merely forgot to emit tasks would
// silently terminate (and, in structured hosts, commit) a half-finished turn.
// Completion is an explicit act — `next_action: "finalize"` — and the schema now
// makes "no action" impossible to emit rather than merely rejected.
//
// parseReplanResult normalises the result the same way parsePlanResult does.
type ReplanResult struct {
	// Message is the replan round's counterpart of PlanResult.Message, kept for the
	// same reason and carrying the same warning: no code reads it, it survives in
	// the run's record, and it is not user-facing.
	Message string `json:"message,omitempty"`
	// NextAction is PlanResult.NextAction's counterpart.
	NextAction string        `json:"next_action,omitempty"`
	Tasks      []TaskPlan    `json:"tasks,omitempty"`
	Question   *Question     `json:"question,omitempty"`
	Finalize   *FinalizePlan `json:"finalize,omitempty"`
}

// FinalizePlan is the planner's explicit "I'm done" declaration. It carries an
// optional short rationale; the actual user-visible output is produced by the
// entry point (final text, or the validated structured object) after the loop
// exits.
type FinalizePlan struct {
	// Reason is a 1-sentence rationale for terminating now (optional).
	Reason string `json:"reason,omitempty"`
}

// Question is the host-facing payload when the planner needs human input.
// proposal forwards it to the Slack question UI; the job host has
// AllowQuestion=false and therefore never sees one.
type Question struct {
	// Reason is the rationale shared across every item ("why am I
	// asking?").
	Reason string `json:"reason"`
	// Items is the ordered list of questions to ask (1..5 items,
	// enforced by Validate).
	Items []QuestionItem `json:"items"`
}

// QuestionItemType discriminates how the host should render the answer
// control. Closed-list types (select / multi_select) require non-empty
// Options; free_text is the last-resort prose-input shape and Options is
// ignored. The planner is told to prefer the closed-list types — see
// prompts/planner.md for the policy.
type QuestionItemType string

const (
	QuestionItemSelect      QuestionItemType = "select"
	QuestionItemMultiSelect QuestionItemType = "multi_select"
	QuestionItemFreeText    QuestionItemType = "free_text"
)

// QuestionItem is one question within Question.Items.
type QuestionItem struct {
	ID      string           `json:"id"`
	Text    string           `json:"text"`
	Type    QuestionItemType `json:"type"`
	Options []string         `json:"options,omitempty"`
}

// QuestionAnswer is the host's reply payload for one QuestionItem.
type QuestionAnswer struct {
	ID       string   `json:"id"`
	Choice   string   `json:"choice,omitempty"`    // select
	Choices  []string `json:"choices,omitempty"`   // multi_select
	FreeText string   `json:"free_text,omitempty"` // free_text
}

// QuestionResult is what the host returns from OnQuestion. The
// hecatoncheires proposal host uses the {Terminate=true} shape (the
// session ends after the planner asks) but the type is set up to also
// support warren-style in-loop continuation in the future.
type QuestionResult struct {
	// Terminate, when true, signals planexec.Runner to stop the loop
	// immediately and return RunStatus=Completed without invoking the
	// final-response phase. Used by proposal to defer the conversation
	// to the next thread reply.
	Terminate bool
	// Items, when Terminate=false, supplies the user's answers and the
	// loop continues with these injected into the next planner round.
	// Empty when Terminate=true.
	Items []QuestionAnswer
}

// Validate enforces TaskPlan invariants. KnownToolIDs is the
// host-supplied allowlist (RunRequest.KnownToolIDs); every entry in
// TaskPlan.Tools must be a member.
// Validate checks one task's own shape. The per-task budget is NOT checked here:
// its rules are stated against the round's remaining allowance, so they live in
// validateTaskList where that figure is in hand.
func (t *TaskPlan) Validate(knownToolIDs []string) error {
	if t == nil {
		return goerr.New("task plan is nil")
	}
	if strings.TrimSpace(t.ID) == "" {
		return goerr.New("task plan id is required")
	}
	if strings.TrimSpace(t.Title) == "" {
		return goerr.New("task plan title is required",
			goerr.V("task_id", t.ID))
	}
	if strings.TrimSpace(t.Description) == "" {
		return goerr.New("task plan description is required",
			goerr.V("task_id", t.ID))
	}
	if strings.TrimSpace(t.AcceptanceCriteria) == "" {
		return goerr.New("task plan acceptance criteria is required",
			goerr.V("task_id", t.ID))
	}
	if len(t.Tools) == 0 {
		return goerr.New("task plan tools must not be empty",
			goerr.V("task_id", t.ID))
	}
	if len(t.Tools) > maxToolsPerTask {
		return goerr.New("task plan tools too many entries",
			goerr.V("task_id", t.ID),
			goerr.V("got", len(t.Tools)),
			goerr.V("max", maxToolsPerTask))
	}
	for _, id := range t.Tools {
		if !slices.Contains(knownToolIDs, id) {
			return goerr.New("task plan tools contains unknown id",
				goerr.V("task_id", t.ID),
				goerr.V("tool_id", id),
				goerr.V("known", knownToolIDs))
		}
	}
	return nil
}

// Validate enforces DirectPlan invariants. knownToolIDs is the host-supplied
// allowlist (RunRequest.KnownToolIDs); every entry in Tools must be a member.
// An empty Tools list is allowed — a direct reply need not call any tool.
func (d *DirectPlan) Validate(knownToolIDs []string) error {
	if d == nil {
		return goerr.New("direct plan is nil")
	}
	if len(d.Tools) > maxToolsPerTask {
		return goerr.New("direct plan tools too many entries",
			goerr.V("got", len(d.Tools)),
			goerr.V("max", maxToolsPerTask))
	}
	for _, id := range d.Tools {
		if !slices.Contains(knownToolIDs, id) {
			return goerr.New("direct plan tools contains unknown id",
				goerr.V("tool_id", id),
				goerr.V("known", knownToolIDs))
		}
	}
	return nil
}

// Validate enforces Question invariants. Called from Validate on the
// containing ReplanResult.
func (q *Question) Validate() error {
	if q == nil {
		return goerr.New("question is nil")
	}
	if strings.TrimSpace(q.Reason) == "" {
		return goerr.New("question reason is required")
	}
	if n := len(q.Items); n < minQuestionItems || n > maxQuestionItems {
		return goerr.New("question items count out of range",
			goerr.V("got", n),
			goerr.V("min", minQuestionItems),
			goerr.V("max", maxQuestionItems))
	}
	seenID := make(map[string]struct{}, len(q.Items))
	for i := range q.Items {
		item := &q.Items[i]
		if err := item.Validate(); err != nil {
			return goerr.Wrap(err, "question item invalid",
				goerr.V("index", i))
		}
		if _, dup := seenID[item.ID]; dup {
			return goerr.New("question item id duplicated",
				goerr.V("id", item.ID))
		}
		seenID[item.ID] = struct{}{}
	}
	return nil
}

// Validate enforces QuestionItem invariants. The free_text exemption
// applies: free_text items skip the Options ≥2 rule and ignore any
// supplied options as a discardable hint.
func (i *QuestionItem) Validate() error {
	if i == nil {
		return goerr.New("question item is nil")
	}
	if strings.TrimSpace(i.ID) == "" {
		return goerr.New("question item id is required")
	}
	if strings.TrimSpace(i.Text) == "" {
		return goerr.New("question item text is required",
			goerr.V("id", i.ID))
	}
	switch i.Type {
	case QuestionItemSelect, QuestionItemMultiSelect:
		if len(i.Options) < minQuestionOptions {
			return goerr.New("question item options must contain at least the minimum",
				goerr.V("id", i.ID),
				goerr.V("got", len(i.Options)),
				goerr.V("min", minQuestionOptions))
		}
		seenOpt := make(map[string]struct{}, len(i.Options))
		for j, opt := range i.Options {
			if strings.TrimSpace(opt) == "" {
				return goerr.New("question item option must not be empty",
					goerr.V("id", i.ID), goerr.V("index", j))
			}
			if _, dup := seenOpt[opt]; dup {
				return goerr.New("question item option duplicated",
					goerr.V("id", i.ID), goerr.V("option", opt))
			}
			seenOpt[opt] = struct{}{}
		}
	case QuestionItemFreeText:
		// Options ignored.
	default:
		return goerr.New("question item type must be select, multi_select, or free_text",
			goerr.V("id", i.ID),
			goerr.V("got", string(i.Type)))
	}
	return nil
}

// parsePlanResult decodes and validates a first-round planner JSON. The planner
// takes one of the actions planActions offers:
//   - Tasks (default): non-empty within bounds; every TaskPlan is validated
//     against knownToolIDs.
//   - Direct (fast path): only offered when allowDirect is true. The
//     direct.tools list is validated against knownToolIDs.
//   - Finalize: nothing to investigate or do; the turn goes to its terminal
//     output.
//
// The action is read from `next_action` (see chooseAction), and the result is
// normalised so only the chosen action's payload is left set.
func parsePlanResult(raw []byte, knownToolIDs []string, allowDirect bool,
	remaining *pricing.NanoUSD,
) (*PlanResult, error) {
	body := extractJSONObject(raw)
	var p PlanResult
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, goerr.Wrap(err, "decode plan json")
	}
	action, err := chooseAction(p.NextAction, planActions(allowDirect), map[string]bool{
		nextActionTasks:    len(p.Tasks) > 0,
		nextActionDirect:   p.Direct != nil,
		nextActionFinalize: p.Finalize != nil,
	})
	if err != nil {
		return nil, goerr.Wrap(err, "plan rejected")
	}
	p.NextAction = action

	switch action {
	case nextActionDirect:
		if err := p.Direct.Validate(knownToolIDs); err != nil {
			return nil, goerr.Wrap(err, "direct plan invalid")
		}
		p.Tasks, p.Finalize = nil, nil
	case nextActionFinalize:
		if p.Finalize == nil {
			p.Finalize = &FinalizePlan{}
		}
		p.Tasks, p.Direct = nil, nil
	default:
		if err := validateTaskList(p.Tasks, knownToolIDs, remaining); err != nil {
			return nil, err
		}
		p.Direct, p.Finalize = nil, nil
	}
	return &p, nil
}

// parseReplanResult decodes and validates a subsequent-round planner JSON. The
// replan round takes EXACTLY ONE of the actions replanActions offers:
//   - Tasks (continuation): another phase runs
//   - Question: the host asks the user (only offered when allowQuestion)
//   - Finalize: the planner declares completion → the final output is produced
//
// Taking none is rejected (the caller folds it back into another replan round)
// so a planner that merely omitted tasks cannot silently terminate the turn.
// The action is read from `next_action` (see chooseAction), and the result is
// normalised so only the chosen action's payload is left set.
func parseReplanResult(raw []byte, knownToolIDs []string, allowQuestion bool,
	remaining *pricing.NanoUSD,
) (*ReplanResult, error) {
	body := extractJSONObject(raw)
	var r ReplanResult
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, goerr.Wrap(err, "decode replan json")
	}
	action, err := chooseAction(r.NextAction, replanActions(allowQuestion), map[string]bool{
		nextActionTasks:    len(r.Tasks) > 0,
		nextActionQuestion: r.Question != nil,
		nextActionFinalize: r.Finalize != nil,
	})
	if err != nil {
		return nil, goerr.Wrap(err, "replan rejected")
	}
	r.NextAction = action

	switch action {
	case nextActionQuestion:
		if err := r.Question.Validate(); err != nil {
			return nil, goerr.Wrap(err, "replan question invalid")
		}
		r.Tasks, r.Finalize = nil, nil
	case nextActionFinalize:
		if r.Finalize == nil {
			r.Finalize = &FinalizePlan{}
		}
		r.Tasks, r.Question = nil, nil
	default:
		if err := validateTaskList(r.Tasks, knownToolIDs, remaining); err != nil {
			return nil, err
		}
		r.Question, r.Finalize = nil, nil
	}
	return &r, nil
}

// planActions is what round 1 may do, in the order the schema lists it.
func planActions(allowDirect bool) []string {
	if allowDirect {
		return []string{nextActionTasks, nextActionDirect, nextActionFinalize}
	}
	return []string{nextActionTasks, nextActionFinalize}
}

// replanActions is what every later round may do, in the order the schema
// lists it.
func replanActions(allowQuestion bool) []string {
	if allowQuestion {
		return []string{nextActionTasks, nextActionQuestion, nextActionFinalize}
	}
	return []string{nextActionTasks, nextActionFinalize}
}

// chooseAction decides which action a planner output takes.
//
// A declared `next_action` decides it. It must be one of offered, and an action
// that carries a payload (tasks / direct / question) must have that payload;
// payloads of the actions NOT chosen are ignored, because the declaration is the
// planner's stated intent and the schema cannot stop a model from filling an
// optional sibling as well.
//
// With no declaration — a provider that did not honour the schema, or a test
// fixture written before the field existed — the action is inferred from which
// payloads are set, and exactly one must be.
//
// Every message is written for the planner, which is shown it verbatim
// (planRetryInput): each one says what to emit instead, and the ones a planner
// that has decided to do nothing would hit point it at finalize. Rejections that
// only said "provide exactly one of tasks, question, or finalize" were what a
// planner with nothing to do kept hitting until it escaped through a tool call.
func chooseAction(declared string, offered []string, present map[string]bool) (string, error) {
	choices := `"` + strings.Join(offered, `", "`) + `"`
	if declared != "" {
		if !slices.Contains(offered, declared) {
			return "", goerr.New(fmt.Sprintf("next_action %q is not available here; set it to one of %s", declared, choices))
		}
		if declared == nextActionFinalize || present[declared] {
			return declared, nil
		}
		if declared == nextActionTasks {
			return "", goerr.New(`next_action is "tasks" but tasks is empty; if there is nothing to investigate or do, set next_action to "finalize" instead`)
		}
		return "", goerr.New(fmt.Sprintf("next_action is %q but the %q payload is missing; fill it, or choose another action", declared, declared))
	}

	var set []string
	for _, a := range offered {
		if present[a] {
			set = append(set, a)
		}
	}
	for a, on := range present {
		if on && !slices.Contains(offered, a) {
			return "", goerr.New(fmt.Sprintf("%q is not available here; set next_action to one of %s", a, choices))
		}
	}
	switch len(set) {
	case 1:
		return set[0], nil
	case 0:
		return "", goerr.New(fmt.Sprintf(`no action was taken; set next_action to one of %s — if there is nothing to investigate or do, set it to "finalize"`, choices))
	default:
		return "", goerr.New(fmt.Sprintf("more than one action was set (%s); set next_action to exactly one of them", strings.Join(set, ", ")))
	}
}

// validateTaskList checks one round's tasks.
//
// remaining is what the run may still spend, or nil when the host wired no
// Config.Remaining and per-task budgets are therefore not in play. It is a
// POINTER rather than a zero-means-unchecked figure because zero is a legitimate
// remaining allowance, and reading it as "do not check" would silently drop the
// check exactly when it matters most.
func validateTaskList(tasks []TaskPlan, knownToolIDs []string, remaining *pricing.NanoUSD) error {
	if n := len(tasks); n < minTasksPerPhase || n > maxTasksPerPhase {
		return goerr.New("tasks count out of range",
			goerr.V("got", n),
			goerr.V("min", minTasksPerPhase),
			goerr.V("max", maxTasksPerPhase))
	}
	seenID := make(map[string]struct{}, len(tasks))
	var sum pricing.NanoUSD
	for i := range tasks {
		task := &tasks[i]
		if err := task.Validate(knownToolIDs); err != nil {
			return goerr.Wrap(err, "task invalid",
				goerr.V("index", i))
		}
		if _, dup := seenID[task.ID]; dup {
			return goerr.New("task id duplicated within plan",
				goerr.V("id", task.ID))
		}
		seenID[task.ID] = struct{}{}
		if remaining == nil {
			continue
		}
		if err := checkTaskBudget(task, *remaining); err != nil {
			return err
		}
		sum += pricing.FromUSD(task.BudgetUSD)
	}
	// The per-task check bounds one task; this bounds the ROUND. Without it a
	// planner could hand every one of five tasks the whole remaining allowance.
	//
	// The sum cannot overflow: checkTaskBudget has already bounded every term by
	// the remaining allowance, and there are at most maxTasksPerPhase of them.
	if remaining != nil && sum > *remaining {
		return goerr.New("task budgets exceed the remaining allowance",
			goerr.V("sum", sum.USD()),
			goerr.V("remaining", remaining.USD()))
	}
	return nil
}

// checkTaskBudget holds the whole per-task budget contract, in the units it is
// actually enforced in.
//
// Both bounds are stated against the CONVERTED amount rather than the float the
// model emitted, and that is the point of the function. `budget_usd` is a float64
// from a model, and pricing.FromUSD does not saturate: a figure above roughly
// 9.2e9 converts outside int64 and lands negative. A positive-float check alone
// would pass it, the round's sum would go negative and clear the sum check, and
// WithBudget would then read the non-positive amount as "unset" and hand the
// child the deployment default — an invalid input granting MORE money than a
// valid one. Bounding the converted amount above closes that direction and bounds
// the sum at the same time.
func checkTaskBudget(task *TaskPlan, remaining pricing.NanoUSD) error {
	amount := pricing.FromUSD(task.BudgetUSD)
	if amount <= 0 {
		// Rejected rather than defaulted: a task with no allowance would inherit the
		// run's whole budget, which is the state this field exists to end.
		return goerr.New("task plan budget must be a positive amount in USD",
			goerr.V("task_id", task.ID),
			goerr.V("budget_usd", task.BudgetUSD))
	}
	if amount > remaining {
		return goerr.New("task plan budget exceeds the remaining allowance",
			goerr.V("task_id", task.ID),
			goerr.V("budget", amount.USD()),
			goerr.V("remaining", remaining.USD()))
	}
	return nil
}

// extractJSONObject returns the slice of raw containing a single
// top-level JSON object. We strip a ```json fence (or bare ``` fence)
// if present and then scan for the first balanced `{ … }` region. The
// scanner is the single source of truth for "which substring is the
// object" — an earlier first-and-last-char fast path silently returned
// the whole input for pathological cases like `{"a":1} {"b":2}` and
// pushed the json.Unmarshal failure downstream instead of letting the
// scanner extract just the first object. (Inherited verbatim from the
// proposal-side implementation; see draft/plan.go's prior version.)
func extractJSONObject(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}
	if bytes.HasPrefix(trimmed, []byte("```")) {
		body := trimmed[3:]
		if rest, ok := bytes.CutPrefix(body, []byte("json")); ok {
			body = rest
		}
		body = bytes.TrimLeft(body, "\r\n")
		if end := bytes.LastIndex(body, []byte("```")); end >= 0 {
			body = bytes.TrimSpace(body[:end])
		}
		trimmed = body
	}
	start := bytes.IndexByte(trimmed, '{')
	if start < 0 {
		return raw
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(trimmed); i++ {
		c := trimmed[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1]
			}
		}
	}
	return raw
}

// schemaOptions controls which sections appear in the planner schema for
// a given round. The first round uses planSchema (no question), every
// subsequent round uses replanSchema (question optional via
// AllowQuestion).
type schemaOptions struct {
	knownToolIDs  []string
	allowQuestion bool
	allowDirect   bool
	// withBudget adds the per-task budget property. It follows Config.Remaining:
	// a host that supplies no remaining figure is not asking the planner to
	// allocate anything, and offering the property anyway would invite a number
	// nothing reads.
	withBudget bool
}

// planSchema returns the gollem.Parameter applied to the planner LLM's
// JSON output for the FIRST round. `tasks` and `finalize` are always offered;
// `direct` is added only when the host opted in (allowDirect). Which one the
// output takes is declared by the required `next_action`; the payload checks and
// the task-count bounds are enforced Go-side in parsePlanResult.
func planSchema(opts schemaOptions) *gollem.Parameter {
	actions := planActions(opts.allowDirect)
	props := map[string]*gollem.Parameter{
		"message":     messageSchema(),
		"next_action": nextActionSchema(actions),
		"tasks":       tasksSchema(opts.knownToolIDs, opts.withBudget),
		"finalize":    finalizeSchema(),
	}
	if opts.allowDirect {
		props["direct"] = directSchema(opts.knownToolIDs)
	}
	return &gollem.Parameter{
		Type: gollem.TypeObject,
		Description: "Initial planner output. Set `next_action` to the action you take (" +
			strings.Join(actions, " / ") + ") and fill that action's field. " +
			"`finalize` means there is nothing to investigate or do.",
		Properties: props,
	}
}

// directSchema returns the gollem.Parameter for the round-1 `direct` payload.
// Only `tools` (a subset of the known tool ids) is carried; the rest of the
// direct path is runtime-supplied.
func directSchema(knownToolIDs []string) *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Answer the user directly without any investigation phase. Mutually exclusive with `tasks`; set only one.",
		Properties: map[string]*gollem.Parameter{
			"tools": {
				Type:        gollem.TypeArray,
				Description: "Tool ids the direct agent may call (0-4). Omit or leave empty for a pure conversational reply.",
				Items: &gollem.Parameter{
					Type: gollem.TypeString,
					Enum: knownToolIDs,
				},
			},
		},
	}
}

// replanSchema returns the gollem.Parameter applied to the planner LLM's
// JSON output for every round AFTER the first. May include `question`
// when the host enabled it.
func replanSchema(opts schemaOptions) *gollem.Parameter {
	actions := replanActions(opts.allowQuestion)
	props := map[string]*gollem.Parameter{
		"message":     messageSchema(),
		"next_action": nextActionSchema(actions),
		"tasks":       tasksSchema(opts.knownToolIDs, opts.withBudget),
		"finalize":    finalizeSchema(),
	}
	if opts.allowQuestion {
		props["question"] = questionSchema()
	}
	return &gollem.Parameter{
		Type: gollem.TypeObject,
		Description: "Replan output. Set `next_action` to the ONE action you take (" +
			strings.Join(actions, " / ") + ") and fill that action's field: `tasks` runs another " +
			"phase, `question` asks the user, `finalize` declares the turn complete.",
		Properties: props,
	}
}

// nextActionSchema is the `next_action` property: the planner's declaration of
// which action its output takes, from the actions this round offers.
//
// It is Required, and that is the whole point of it. The actions themselves are
// optional siblings (requiring them would make the planner fill every one), so
// with only those a structured output could emit NO action — and a planner that
// had decided there was nothing to do had no action that said so on round 1.
// Those outputs were rejected and re-asked, and under structured outputs the one
// way out of the schema is a tool call: production planners answered the
// rejections with tool calls carrying placeholder arguments ("x", "noop",
// "dummy", tools named notion__none), and when the call they reached for was
// slack__post_to_case_channel the placeholder was posted to a customer thread. A
// required enum makes "no action" impossible to emit, and `finalize` is in the
// enum on every round, so "nothing to do" always has a value.
//
// The name sorts after `message`, which matters for the reason messageSchema
// gives: Claude emits required properties first, in schema (alphabetical) order,
// so the planner writes its rationale and THEN commits to an action, and every
// action's payload still follows. Only offered actions are in the enum — one the
// parser would reject must not be emittable.
func nextActionSchema(actions []string) *gollem.Parameter {
	return &gollem.Parameter{
		Type: gollem.TypeString,
		Description: "The action this output takes. Fill the field of the same name (except for " +
			"`finalize`, whose field is optional). Choose `finalize` when there is nothing (more) " +
			"to investigate or do.",
		Required: true,
		Enum:     actions,
	}
}

// messageSchema is the `message` property shared by planSchema and replanSchema.
//
// It is Required because of the order Claude's structured outputs emit
// properties in: required properties first, then optional ones, each group in
// schema order — and the schema gollem sends has its properties sorted
// alphabetically. With every top-level property optional, a replan was emitted
// as `finalize` → `message` → `question` → `tasks`, so once the planner had
// started writing its rationale `finalize` could no longer be emitted (in the
// plan schema, `direct` likewise precedes `message`). A planner whose `message`
// said it was finishing then satisfied "set exactly one action" with the only
// action still ahead of it: a placeholder `question` (reason "x", options
// "a" / "b") that Question.Validate accepts and the host posts to the thread.
// Required puts `message` first, so every action is still reachable after it.
//
// The actions stay optional: requiring them would make the planner fill every
// one of them, which breaks "set exactly one". Which one is set is declared by
// the other required property, `next_action` (see nextActionSchema), which sorts
// after `message` so this ordering argument still holds.
func messageSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeString,
		Description: rationaleDescription,
		Required:    true,
	}
}

// finalizeSchema is the `finalize` action shape: an explicit completion
// declaration carrying only an optional rationale.
func finalizeSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Optional rationale when `next_action` is `finalize`: the turn is complete (or there was nothing to do) and the final output is produced next. An empty `tasks` list alone does NOT signal completion.",
		Properties: map[string]*gollem.Parameter{
			"reason": {
				Type:        gollem.TypeString,
				Description: "Optional 1-sentence rationale for finishing now.",
			},
		},
	}
}

func tasksSchema(knownToolIDs []string, withBudget bool) *gollem.Parameter {
	// Every field below is enforced by TaskPlan.Validate; mark them Required so
	// the model is compelled to emit them. Without this, models that omit (most
	// commonly) `description` send the planner into a retry loop that burns the
	// whole budget before the turn falls back.
	taskProps := map[string]*gollem.Parameter{
		"id":                  {Type: gollem.TypeString, Description: "Phase-unique identifier (e.g. inv-1).", Required: true},
		"title":               {Type: gollem.TypeString, Description: "Short label for the trace UI (~40 chars).", Required: true},
		"description":         {Type: gollem.TypeString, Description: "Detailed instruction for the sub-agent.", Required: true},
		"acceptance_criteria": {Type: gollem.TypeString, Description: "Measurable completion condition.", Required: true},
		"tools": {
			Type:        gollem.TypeArray,
			Description: "Allowed tool set IDs for this task.",
			Required:    true,
			Items: &gollem.Parameter{
				Type: gollem.TypeString,
				Enum: knownToolIDs,
			},
		},
	}
	if withBudget {
		// Required for the same reason every field above is: a model that omits it
		// sends the round into a retry loop, and the retries are themselves paid for
		// out of the allowance being divided.
		taskProps["budget_usd"] = &gollem.Parameter{
			Type: gollem.TypeNumber,
			Description: "How much of the run's remaining allowance, in USD, this task's " +
				"sub-agent may spend. Must be greater than 0, and the budgets of all tasks " +
				"in this round must add up to no more than the remaining allowance stated " +
				"in your instructions. Give the heavier task the larger share.",
			Required: true,
		}
	}
	return &gollem.Parameter{
		Type:        gollem.TypeArray,
		Description: "Parallel investigation tasks for this phase.",
		Items: &gollem.Parameter{
			Type:       gollem.TypeObject,
			Properties: taskProps,
		},
	}
}

func questionSchema() *gollem.Parameter {
	// Mirror tasksSchema: mark every field Question.Validate enforces as Required
	// and bound `items` to 1..maxQuestionItems, so the provider's structured
	// output is compelled to emit a well-formed, NON-EMPTY question. Without this
	// the model can emit an empty `items` array (or an item missing id/text/type),
	// which parseReplanResult rejects post-hoc and retries — and, exactly like the
	// task path before it was constrained, a model that keeps emitting an empty
	// question burns the whole planner budget before the turn falls back. `options`
	// stays optional because free_text items legitimately omit it.
	minItems := minQuestionItems
	maxItems := maxQuestionItems
	itemProps := map[string]*gollem.Parameter{
		"id":   {Type: gollem.TypeString, Description: "Item-unique identifier (e.g. q-1).", Required: true},
		"text": {Type: gollem.TypeString, Description: "Question text shown to the user.", Required: true},
		"type": {
			Type:        gollem.TypeString,
			Description: "Answer control type. Use free_text only as a last resort.",
			Required:    true,
			Enum: []string{
				string(QuestionItemSelect),
				string(QuestionItemMultiSelect),
				string(QuestionItemFreeText),
			},
		},
		"options": {
			Type:        gollem.TypeArray,
			Description: "Allowed answer values for select / multi_select (≥2 entries). Ignored for free_text.",
			Items:       &gollem.Parameter{Type: gollem.TypeString},
		},
	}
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Ask the user for clarification. Use sparingly — prefer continuing investigation when possible.",
		Properties: map[string]*gollem.Parameter{
			"reason": {Type: gollem.TypeString, Description: "Why these questions are necessary.", Required: true},
			"items": {
				Type:        gollem.TypeArray,
				Description: "Ordered list of questions (1-5). MUST contain at least one item — never an empty list.",
				Required:    true,
				MinItems:    &minItems,
				MaxItems:    &maxItems,
				Items: &gollem.Parameter{
					Type:       gollem.TypeObject,
					Properties: itemProps,
				},
			},
		},
	}
}
