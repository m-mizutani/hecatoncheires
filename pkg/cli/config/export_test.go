package config

import (
	"context"

	"github.com/slack-go/slack"

	agentkernel "github.com/m-mizutani/hecatoncheires/pkg/agent/kernel"
)

// NewSlackForTest creates a Slack config for testing purposes
func NewSlackForTest(clientID, clientSecret, botToken, signingSecret, noAuthUID string) *Slack {
	return &Slack{
		clientID:      clientID,
		clientSecret:  clientSecret,
		botToken:      botToken,
		signingSecret: signingSecret,
		noAuthUID:     noAuthUID,
	}
}

// SetOrgLevelForTest sets org-level detection results for testing purposes
func (x *Slack) SetOrgLevelForTest(isOrgLevel bool, authTeamID string) {
	x.isOrgLevel = isOrgLevel
	x.authTeamID = authTeamID
}

// MockSlackAuthAPI is a test double for slackAuthAPI
type MockSlackAuthAPI struct {
	AuthTestResp *slack.AuthTestResponse
	AuthTestErr  error
	Teams        []slack.Team
	ListTeamsErr error
}

func (m *MockSlackAuthAPI) AuthTestContext(_ context.Context) (*slack.AuthTestResponse, error) {
	return m.AuthTestResp, m.AuthTestErr
}

func (m *MockSlackAuthAPI) ListTeamsContext(_ context.Context, _ slack.ListTeamsParameters) ([]slack.Team, string, error) {
	return m.Teams, "", m.ListTeamsErr
}

// SetAuthAPIForTest injects a mock slackAuthAPI for testing DetectOrgLevel
func (x *Slack) SetAuthAPIForTest(api *MockSlackAuthAPI) {
	x.authAPI = api
}

// NewLLMForTest creates an LLM config for testing purposes.
func NewLLMForTest(modelRef, openaiAPIKey, claudeAPIKey, geminiProjectID, geminiLocation string) *LLM {
	return &LLM{
		modelRef:        modelRef,
		openaiAPIKey:    openaiAPIKey,
		claudeAPIKey:    claudeAPIKey,
		geminiProjectID: geminiProjectID,
		geminiLocation:  geminiLocation,
	}
}

// ClaudeReasoningForTest reports what claudeReasoningOf decides for def: whether
// adaptive thinking is sent, and the display and effort strings sent ("" for
// none). Both claude builders read this one decision; a Vertex AI client cannot be
// pointed at a fake server (the SDK resolves Google credentials and the Vertex AI
// host itself), so this is where a test can read the values it is given.
func ClaudeReasoningForTest(def agentkernel.ModelDef) (adaptive bool, display, effort string) {
	r := claudeReasoningOf(def)
	return r.adaptive, string(r.display), string(r.effort)
}

// ClaudeOptionCountsForTest reports how many options the Anthropic API builder
// and the Vertex AI builder produce for def, so a test can pin that both add an
// option for the same settings of the decision ClaudeReasoningForTest reports.
func ClaudeOptionCountsForTest(def agentkernel.ModelDef) (api, vertex int) {
	return len(claudeAPIOptions(def)), len(claudeVertexOptions(def))
}

// NewAgentForTest creates an Agent config with the budget flag set as given.
// budgetUSD of 0 is "the flag was not passed", which is what BudgetOr reads.
func NewAgentForTest(budgetUSD float64) *Agent {
	return &Agent{
		maxSteps:            defaultAgentMaxSteps,
		defaultBudgetUSD:    budgetUSD,
		taskMaxSteps:        defaultAgentTaskMaxSteps,
		taskMaxInputTokens:  defaultAgentTaskMaxInputTokens,
		taskMaxOutputTokens: defaultAgentTaskMaxOutputTokens,
		noticeRatio:         defaultAgentNoticeRatio,
	}
}

// NewEmbeddingForTest creates an Embedding config for testing purposes.
func NewEmbeddingForTest(geminiProjectID, geminiLocation, model string) *Embedding {
	return &Embedding{
		geminiProjectID: geminiProjectID,
		geminiLocation:  geminiLocation,
		model:           model,
	}
}

// NewSentryForTest creates a Sentry config for testing.
func NewSentryForTest(dsn, environment, release string) *Sentry {
	return &Sentry{
		dsn:         dsn,
		environment: environment,
		release:     release,
	}
}
