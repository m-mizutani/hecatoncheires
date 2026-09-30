package threadcase_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
)

// scriptedLLM pops canned responses in order; shared between planner and
// sub-agent calls (the order is deterministic).
func newThreadSession() *model.Session {
	return &model.Session{
		ID:          "s-thread-" + time.Now().Format("150405.000000"),
		ChannelID:   "C-MONITOR",
		ThreadTS:    "1700000000.000100",
		WorkspaceID: "support",
		CaseID:      42,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func newThreadWorkspace() *model.WorkspaceEntry {
	set, _ := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
		{ID: "TRIAGE", Name: "Triage"},
		{ID: "DONE", Name: "Done"},
	})
	return &model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		CaseStatusSet:         set,
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	}
}

func newThreadCase() *model.Case {
	return &model.Case{
		ID:             42,
		Title:          "Initial title",
		Status:         types.CaseStatusOpen,
		ReporterID:     "U-REPORTER",
		SlackChannelID: "C-MONITOR",
		SlackThreadTS:  "1700000000.000100",
		BoardStatus:    "TRIAGE",
	}
}

// investigatePlan is the round-1 plan that runs one read-only sub-agent.
// Thread-mode manages no Actions, so the planner is offered no core (action)
// toolset; the read-only Slack toolset stands in.
const investigatePlan = `{"message":"investigate the thread","tasks":[{"id":"t-1","title":"Review thread","description":"Review the message","acceptance_criteria":"reviewed","tools":["slack_ro"],"budget_usd":0.01}]}`

// replanDone terminates the loop. Under the explicit-finalize design an empty
// tasks list no longer signals completion; the planner must emit `finalize`.
const replanDone = `{"message":"enough context","finalize":{"reason":"goal met"}}`

func TestDecision_Validate(t *testing.T) {
	// Unknown kind is rejected.
	gt.Error(t, threadcase.Decision{Kind: "explode"}.Validate())
	// respond requires a non-empty message.
	gt.Error(t, threadcase.Decision{Kind: threadcase.DecisionRespond}.Validate())
	gt.NoError(t, threadcase.Decision{Kind: threadcase.DecisionRespond, Message: "hi"}.Validate())
	// materialize requires both title and description.
	gt.Error(t, threadcase.Decision{Kind: threadcase.DecisionMaterialize, Title: "t"}.Validate())
	gt.NoError(t, threadcase.Decision{Kind: threadcase.DecisionMaterialize, Title: "t", Description: "d"}.Validate())
}

// createWorkspaceEntry is the workspace used by the ModeCreate tests: it has a
// required select (severity) and a required text (summary).
func createTestWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
				{ID: "summary", Name: "Summary", Type: types.FieldTypeText, Required: true},
			},
		},
	}
}
