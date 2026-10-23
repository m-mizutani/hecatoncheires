package planexec_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/hecatoncheires/pkg/usecase/agent/planexec"
)

func TestPlannerPrompt_HappyPath(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "You are the proposal planner for case drafts.",
		Language:          "Japanese",
		KnownToolIDs:      []string{"slack_ro", "github"},
		AllowQuestion:     true,
		StructuredFinal:   true,
	})
	gt.NoError(t, err).Required()
	// HostPrompt is prepended verbatim.
	gt.String(t, got).Contains("You are the proposal planner for case drafts.")
	// Loop description section.
	gt.String(t, got).Contains("Planner protocol")
	gt.String(t, got).Contains("Round 1")
	gt.String(t, got).Contains("Round 2 and later")
	// AllowQuestion = true → question section appears.
	gt.String(t, got).Contains("question")
	gt.String(t, got).Contains("ask the user")
	// StructuredFinal = true → structured-final mention.
	gt.String(t, got).Contains("structured JSON")
	// Language directive references the supplied language.
	gt.String(t, got).Contains("Japanese")
	// Known tool IDs are listed.
	gt.String(t, got).Contains("`slack_ro`")
	gt.String(t, got).Contains("`github`")
}

func TestPlannerPrompt_QuestionDisabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "Run scheduled job analysis.",
		KnownToolIDs:      []string{"default"},
		AllowQuestion:     false,
		StructuredFinal:   false,
	})
	gt.NoError(t, err).Required()
	// AllowQuestion = false → no "ask the user" section.
	gt.String(t, got).Contains("Run scheduled job analysis.")
	// The "question" word will still appear in the Loop shape's
	// "neither" sentence, but the bullet describing question itself
	// should NOT be present. We assert on the action verb "ask the
	// user" which is unique to the conditional block.
	gt.Bool(t, contains(got, "ask the user")).False()
	// StructuredFinal = false → "plain text" mention.
	gt.String(t, got).Contains("plain text")
}

func TestPlannerPrompt_LanguageOmitted(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "x",
		KnownToolIDs:      []string{"a"},
		AllowQuestion:     false,
		// Language: ""
	})
	gt.NoError(t, err).Required()
	// No "MUST be written in" directive when language is empty.
	gt.Bool(t, contains(got, "MUST be written in")).False()
}

func TestPlannerPrompt_DirectEnabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "You are the thread planner.",
		KnownToolIDs:      []string{"slack_ro"},
		AllowQuestion:     true,
		AllowDirect:       true,
	})
	gt.NoError(t, err).Required()
	// The Direct answer section and its strict guard appear.
	gt.String(t, got).Contains("Direct answer")
	gt.String(t, got).Contains("`direct`")
	gt.String(t, got).Contains("When in ANY doubt")
}

func TestPlannerPrompt_DirectDisabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "You are the thread planner.",
		KnownToolIDs:      []string{"slack_ro"},
		AllowQuestion:     true,
		// AllowDirect: false
	})
	gt.NoError(t, err).Required()
	// No Direct answer section when the host did not opt in.
	gt.Bool(t, contains(got, "Direct answer")).False()
}

func TestPlannerPrompt_SubAgentWritesEnabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds:   4,
		HostPrompt:          "Run scheduled job analysis.",
		KnownToolIDs:        []string{"default"},
		AllowSubAgentWrites: true,
	})
	gt.NoError(t, err).Required()
	// The "Actions and writes" guidance and its key rules appear.
	gt.String(t, got).Contains("Actions and writes")
	gt.String(t, got).Contains("investigate first")
	gt.String(t, got).Contains("self-evident")
	gt.String(t, got).Contains("same phase")
	gt.String(t, got).Contains("final response cannot perform actions")
}

func TestPlannerPrompt_SubAgentWritesDisabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "You are the thread planner.",
		KnownToolIDs:      []string{"slack_ro"},
		// AllowSubAgentWrites: false
	})
	gt.NoError(t, err).Required()
	// No write guidance when the host did not opt in.
	gt.Bool(t, contains(got, "Actions and writes")).False()
}

// The prompt must tell the planner both things about `message`, because each half
// prevents a different failure.
//
// It is NOT user-facing: no code reads it (the Sink that once delivered it was
// removed with the in-process Runner), so a planner that believes it is writing to
// the user there writes the answer into a field that is discarded, and the turn
// ends with the reply nowhere.
//
// It IS kept: the planner reply carrying it is committed to the run's conversation
// and recorded as that transition's LLM_RESPONSE, which is where an operator reads
// why a turn decided as it did. Told only "nobody reads this", a planner has every
// reason to emit a placeholder and that record goes empty.
func TestPlannerPrompt_MessageIsInternalButKeptInTheRunRecord(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "You are the thread planner.",
		Language:          "Japanese",
		KnownToolIDs:      []string{"slack_ro"},
		AllowQuestion:     true,
	})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("`message` is NOT shown to the user")
	gt.String(t, got).Contains("kept in this run's record")
	// The language directive still names the fields the user DOES read, and no
	// longer names `message`.
	gt.String(t, got).Contains("MUST be written in **Japanese**")
	gt.String(t, got).NotContains("the `message` field, ")
}

// The planner may call tools itself, but it is declared only read tools (see
// kernel.NewToolFactory). The protocol must say so, together with where writes
// go — a task carrying the write tool id — so the prompt and the wiring describe
// the same capability. `finalize` is offered in round 1 for a request whose
// lookups show there is nothing to do; without it the planner had no valid
// output there and filled the plan with placeholder tasks to pass validation.
func TestPlannerPrompt_OwnToolCallsAndRoundOneFinalize(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds:   4,
		HostPrompt:          "Run scheduled job analysis.",
		KnownToolIDs:        []string{"slack_ro"},
		AllowSubAgentWrites: true,
	})
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("## Your own tool calls")
	gt.String(t, got).Contains("for up to 4 rounds per decision")
	gt.String(t, got).Contains("The tools declared to you only read.")
	gt.String(t, got).Contains("Every such write is a task that carries the tool id granting it.")
	gt.String(t, got).Contains("it is not work this turn has performed")
	gt.String(t, got).Contains("stop calling tools and set `finalize` in the decision object")
	gt.String(t, got).NotContains("finalize_turn")
	gt.String(t, got).NotContains("substitute for a decision")

	// Round 1 offers finalize, not only tasks / direct.
	round1, _, ok := strings.Cut(got, "**Round 2 and later**")
	gt.Bool(t, ok).True().Required()
	gt.String(t, round1).Contains("**`finalize`** — end the turn without running any phase")

	// The final call is described as it behaves: it may still read (stepFinal
	// answers a lookup it asks for) but changes nothing, and every change a tool
	// makes goes through a task. A host-applied terminal output (threadcase's
	// materialize) is not a tool's change, so the sentence leaves it alone.
	gt.String(t, got).Contains("It performs no action itself")
	gt.String(t, got).Contains("That call can still look something up with the read tools, but it changes nothing.")
	gt.String(t, got).NotContains("with no tools")
	gt.String(t, got).NotContains("generate the user-visible output")
	gt.String(t, got).NotContains("produces an internal summary only")
	gt.String(t, got).Contains("Every change a tool makes must have happened before you finalize, through a task carrying the appropriate write tool;")
	gt.String(t, got).NotContains("your own tool call;")
}

func TestPlannerPrompt_RejectsNonPositiveToolRounds(t *testing.T) {
	_, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:   "x",
		KnownToolIDs: []string{"a"},
	})
	gt.Error(t, err)
}

func TestPlannerPrompt_RejectsEmptyHostPrompt(t *testing.T) {
	_, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		KnownToolIDs:      []string{"a"},
	})
	gt.Error(t, err)
}

func TestPlannerPrompt_RejectsEmptyKnownToolIDs(t *testing.T) {
	_, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		PlannerToolRounds: 4,
		HostPrompt:        "x",
	})
	gt.Error(t, err)
}

// contains is a thin substring check used by the negative-presence
// assertions above. We deliberately use a helper here rather than
// gt.String(...).NotContains because the test asserts the *absence*
// of a sub-string with a custom failure message that names what we
// expected to be missing.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
