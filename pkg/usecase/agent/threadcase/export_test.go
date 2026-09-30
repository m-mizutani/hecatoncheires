package threadcase

import "github.com/secmon-lab/hecatoncheires/pkg/domain/model"

// Test-only seams for unit-testing the unexported prompt / decision helpers
// without exporting them into the production API.
var (
	BuildUserInputForTest         = buildUserInput
	ValidateCreateDecisionForTest = validateCreateDecision
	ValidateRequestForTest        = validateRequest
)

// BuildSystemPromptForTest renders the system prompt, failing loudly on a
// template error so call sites can stay single-valued.
func BuildSystemPromptForTest(c *model.Case, ws *model.WorkspaceEntry, mode Mode, createInstruction string) string {
	s, err := buildSystemPrompt(c, ws, mode, createInstruction)
	if err != nil {
		panic(err)
	}
	return s
}

// CreateDecisionForTest re-exports the create decision struct for tests.
type CreateDecisionForTest = CreateDecision
