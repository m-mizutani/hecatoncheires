package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestRenderSummaryFields_Semantic(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		FieldSchema: &config.FieldSchema{Fields: []config.FieldDefinition{
			{ID: "channel", Name: "Notify", Type: types.FieldTypeText, Semantic: types.SemanticSlackChannelID},
			{ID: "legacy", Name: "Legacy", Type: types.FieldTypeText, Semantic: types.SemanticSlackChannelID},
			{ID: "note", Name: "Note", Type: types.FieldTypeText},
		}},
	}
	c := &model.Case{FieldValues: map[string]model.FieldValue{
		"channel": {FieldID: "channel", Type: types.FieldTypeText, Value: "C0123ABCD"},
		"legacy":  {FieldID: "legacy", Type: types.FieldTypeText, Value: "#old"},
		"note":    {FieldID: "note", Type: types.FieldTypeText, Value: "C0123ABCD"},
	}}

	gt.Equal(t, usecase.RenderSummaryFieldsForTest(c, entry), []string{
		"• *Notify*: <#C0123ABCD>",
		// A stored value that does not fit the semantic keeps its raw rendering.
		"• *Legacy*: #old",
		"• *Note*: C0123ABCD",
	})
}
