package graphql

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ToGraphQLCaseForTest exposes the unexported toGraphQLCase converter so the
// external graphql_test package can assert the domain → GraphQL field mapping
// (notably the empty-ReporterID → nil-pointer rule for reporterless thread-mode
// cases).
var ToGraphQLCaseForTest = toGraphQLCase

// ToGraphQLCaseJobForTest exposes the unexported toGraphQLCaseJob converter so
// the external graphql_test package can assert the Job definition → GraphQL
// mapping (strategy normalisation, trigger shape, schedule mutual exclusion).
var ToGraphQLCaseJobForTest = toGraphQLCaseJob

// ToGraphQLJobRunEventForTest exposes the unexported toGraphQLJobRunEvent
// converter so the external graphql_test package can assert the payload JSON
// the run-detail UI and the exported run file both read.
var ToGraphQLJobRunEventForTest = toGraphQLJobRunEvent

// ToGraphQLJobRunEventsForTest exposes the unexported list-level converter so a
// test can assert that a conversation stored as per-call diffs is handed to the
// run-detail UI as whole message lists again.
var ToGraphQLJobRunEventsForTest = toGraphQLJobRunEvents

// ToGraphQLJobRunLogForTest exposes the unexported toGraphQLJobRunLog so a test
// can pin the wire form of a run record — the cost in particular, which is
// stored in nano-USD and read in dollars.
var ToGraphQLJobRunLogForTest = toGraphQLJobRunLog

// ToGraphQLActionCommentForTest exposes the unexported toGraphQLActionComment
// converter so the external graphql_test package can assert the domain →
// GraphQL mapping, notably that `edited` is derived from the timestamps and
// that `author` is left for the dataloader-backed resolver to fill.
var ToGraphQLActionCommentForTest = toGraphQLActionComment

// ToGraphQLFieldTypeForTest exposes the unexported toGraphQLFieldType converter
// so the external graphql_test package can assert the domain → GraphQL field
// type enum bridge (notably the markdown mapping).
var ToGraphQLFieldTypeForTest = toGraphQLFieldType

// TextDisplayKeyForTest names one (semantic, value) pair for
// LoadTextDisplaysForTest.
type TextDisplayKeyForTest struct {
	Semantic types.Semantic
	Value    string
}

// LoadTextDisplaysForTest loads every key in ONE dataloader batch, so a test
// can assert how the batch groups keys by semantic.
func LoadTextDisplaysForTest(ctx context.Context, d *DataLoaders, keys []TextDisplayKeyForTest) ([]*definition.Display, []error) {
	internal := make([]textDisplayKey, len(keys))
	for i, k := range keys {
		internal[i] = textDisplayKey{semantic: k.Semantic, value: k.Value}
	}
	return d.TextDisplay.LoadMany(ctx, internal)()
}

// WithSemanticsForTest exposes the unexported withSemantics converter.
var WithSemanticsForTest = withSemantics

// ToGraphQLFieldValueDisplayForTest exposes the unexported display converter.
var ToGraphQLFieldValueDisplayForTest = toGraphQLFieldValueDisplay
