package semantic_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestValidate(t *testing.T) {
	t.Run("rejects a channel name through the re-exported sentinel", func(t *testing.T) {
		gt.Error(t, semantic.Validate(types.SemanticSlackChannelID, "#general")).Is(semantic.ErrInvalidValue)
	})
	t.Run("accepts a channel ID", func(t *testing.T) {
		gt.NoError(t, semantic.Validate(types.SemanticSlackChannelID, "C0123ABCD"))
	})
	t.Run("accepts an empty value", func(t *testing.T) {
		gt.NoError(t, semantic.Validate(types.SemanticSlackChannelID, ""))
	})
	t.Run("accepts anything when no semantic is set", func(t *testing.T) {
		gt.NoError(t, semantic.Validate("", "#general"))
	})
	t.Run("rejects an unknown semantic", func(t *testing.T) {
		gt.Error(t, semantic.Validate("nope", "x")).Is(semantic.ErrUnknownSemantic)
	})
}

func TestSlackMrkdwn(t *testing.T) {
	channelField := config.FieldDefinition{ID: "ch", Type: types.FieldTypeText, Semantic: types.SemanticSlackChannelID}

	got, ok := semantic.SlackMrkdwn(channelField, "C0123ABCD")
	gt.True(t, ok)
	gt.Value(t, got).Equal("<#C0123ABCD>")

	cases := map[string]struct {
		def   config.FieldDefinition
		value any
	}{
		"no semantic":      {config.FieldDefinition{Type: types.FieldTypeText}, "C0123ABCD"},
		"not a text field": {config.FieldDefinition{Type: types.FieldTypeMarkdown, Semantic: types.SemanticSlackChannelID}, "C0123ABCD"},
		"empty value":      {channelField, ""},
		"invalid value":    {channelField, "#general"},
		"non-string value": {channelField, 42},
		"unknown semantic": {config.FieldDefinition{Type: types.FieldTypeText, Semantic: "nope"}, "C0123ABCD"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, ok := semantic.SlackMrkdwn(tc.def, tc.value)
			gt.False(t, ok)
		})
	}
}

func TestPromptHintAndLabel(t *testing.T) {
	hint := semantic.PromptHint(types.SemanticSlackChannelID)
	gt.True(t, strings.Contains(hint, "C0123456789"))
	gt.True(t, strings.HasPrefix(semantic.Label(types.SemanticSlackChannelID), "slack_channel_id — "))

	gt.Value(t, semantic.PromptHint("")).Equal("")
	gt.Value(t, semantic.PromptHint("nope")).Equal("")
	gt.Value(t, semantic.Label("")).Equal("")
	gt.Value(t, semantic.Label("nope")).Equal("")
}

// TestCatalogEntriesAreComplete walks every registered semantic so a new one
// that is unreachable by name or lacks a prompt hint fails here.
func TestCatalogEntriesAreComplete(t *testing.T) {
	all := semantic.All()
	gt.True(t, len(all) > 0)
	for _, s := range all {
		t.Run(string(s.ID()), func(t *testing.T) {
			found, ok := semantic.Lookup(s.ID())
			gt.True(t, ok)
			gt.Value(t, found.ID()).Equal(s.ID())
			gt.True(t, s.PromptHint() != "")
		})
	}
}
