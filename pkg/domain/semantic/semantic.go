// Package semantic is the catalog of text-field semantics. A semantic names
// what a text value refers to; validation, Slack rendering, the link target,
// the agent prompt hint and the display label all derive from it. Each
// semantic lives in its own subpackage; this package lists them and exposes
// the lookups the rest of the application uses.
package semantic

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/slackchannelid"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

var (
	// ErrInvalidValue is re-exported so callers (the field validator, the
	// error classifiers) depend on this package only.
	ErrInvalidValue    = definition.ErrInvalidValue
	ErrUnknownSemantic = goerr.New("unknown semantic")
)

// catalog lists every semantic. Adding a semantic means adding one entry
// here. It is fixed at compile time and never mutated.
var catalog = []definition.Semantic{
	slackchannelid.New(),
}

// All returns every defined semantic.
func All() []definition.Semantic {
	out := make([]definition.Semantic, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup returns the semantic named id.
func Lookup(id types.Semantic) (definition.Semantic, bool) {
	for _, s := range catalog {
		if s.ID() == id {
			return s, true
		}
	}
	return nil, false
}

// Validate checks value against the semantic named id. An empty id means
// free text and an empty value means "no value"; both pass.
func Validate(id types.Semantic, value string) error {
	if id == "" || value == "" {
		return nil
	}
	s, ok := Lookup(id)
	if !ok {
		return goerr.Wrap(ErrUnknownSemantic, "semantic is not defined",
			goerr.V("semantic", id))
	}
	return s.Validate(value)
}

// PromptHint returns the LLM-facing description of the semantic, or "" for
// an empty or unknown id.
func PromptHint(id types.Semantic) string {
	s, ok := Lookup(id)
	if !ok {
		return ""
	}
	return s.PromptHint()
}

// Label is the one-line form the Job and case-channel prompts render:
// "<id> — <hint>". It is "" for an empty or unknown id.
func Label(id types.Semantic) string {
	hint := PromptHint(id)
	if hint == "" {
		return ""
	}
	return string(id) + " — " + hint
}

// SlackMrkdwn renders value as Slack mrkdwn per the field's semantic. It
// returns ok=false unless def is a text field with a known semantic and value
// is a non-empty string that passes validation, so a stored value that no
// longer fits keeps its existing rendering.
func SlackMrkdwn(def config.FieldDefinition, value any) (string, bool) {
	s, ok := validSemanticValue(def, value)
	if !ok {
		return "", false
	}
	sem, _ := Lookup(def.Semantic)
	return sem.SlackMrkdwn(s), true
}

func validSemanticValue(def config.FieldDefinition, value any) (string, bool) {
	if def.Type != types.FieldTypeText || def.Semantic == "" {
		return "", false
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return "", false
	}
	sem, ok := Lookup(def.Semantic)
	if !ok {
		return "", false
	}
	if err := sem.Validate(s); err != nil {
		return "", false
	}
	return s, true
}
