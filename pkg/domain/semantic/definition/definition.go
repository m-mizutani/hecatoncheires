// Package definition declares the contract every semantic implements. It is
// the leaf of the semantic packages: each semantic package imports it, and
// the catalog in the parent package imports each semantic package, so the
// contract cannot live in the parent without an import cycle.
package definition

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ErrInvalidValue is wrapped by every Semantic.Validate failure.
var ErrInvalidValue = goerr.New("value does not match the semantic")

// Semantic is one interpretation of a text value. Validation, Slack
// rendering, the link target and the prompt hint all derive from it. These
// methods must be free of I/O.
type Semantic interface {
	ID() types.Semantic
	// Validate is never called with an empty string. A failure wraps
	// ErrInvalidValue.
	Validate(value string) error
	// PromptHint is an English sentence telling an LLM what a value looks like.
	PromptHint() string
	// SlackMrkdwn renders a value that passed Validate as Slack mrkdwn.
	SlackMrkdwn(value string) string
	// Link returns the page a value that passed Validate refers to, or ""
	// when the semantic has no page. It is derived from the value alone.
	Link(value string) string
}

// Display is the supplementary information shown under a field value that
// carries a semantic. An empty Label means the name could not be resolved
// (the service is not configured, the lookup found nothing, or the stored
// value does not fit the semantic); clients show that as an error.
type Display struct {
	// Label is what the value refers to, e.g. "#general".
	Label string
	// URL comes from Semantic.Link. It is set only together with Label, so
	// an unresolved value is never linked.
	URL string
}

// Resolver fetches labels for one semantic. It is the only part of a
// semantic that reaches an external service, and it does so solely through
// an interface the semantic's package declares and the caller injects.
type Resolver interface {
	// Resolve receives distinct values that passed Validate. A value without
	// a label is absent from the result.
	Resolve(ctx context.Context, values []string) (map[string]string, error)
}
