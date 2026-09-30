package semantic_test

import (
	"context"
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type fakeChannelNames struct {
	names map[string]string
	err   error
	calls [][]string
}

func (f *fakeChannelNames) GetChannelNames(_ context.Context, ids []string) (map[string]string, error) {
	f.calls = append(f.calls, ids)
	if f.err != nil {
		return nil, f.err
	}
	return f.names, nil
}

func TestDisplayerResolve(t *testing.T) {
	ctx := context.Background()

	t.Run("resolved ids get label and link, unresolved ids an empty display, one deduplicated lookup", func(t *testing.T) {
		lookup := &fakeChannelNames{names: map[string]string{"C1": "general"}}
		d := semantic.NewDisplayer(semantic.Deps{SlackChannelNames: lookup})

		got, err := d.Resolve(ctx, types.SemanticSlackChannelID, []string{"C1", "C2", "C1"})
		gt.NoError(t, err).Required()
		gt.Equal(t, got, map[string]definition.Display{
			"C1": {Label: "#general", URL: "https://slack.com/archives/C1"},
			"C2": {},
		})
		gt.Equal(t, lookup.calls, [][]string{{"C1", "C2"}})
	})

	t.Run("a value that does not fit is unresolved and never reaches the lookup", func(t *testing.T) {
		lookup := &fakeChannelNames{names: map[string]string{}}
		d := semantic.NewDisplayer(semantic.Deps{SlackChannelNames: lookup})

		got, err := d.Resolve(ctx, types.SemanticSlackChannelID, []string{"#general", ""})
		gt.NoError(t, err).Required()
		gt.Equal(t, got, map[string]definition.Display{"#general": {}})
		gt.Equal(t, len(lookup.calls), 0)
	})

	t.Run("without Slack every value is unresolved", func(t *testing.T) {
		d := semantic.NewDisplayer(semantic.Deps{})
		got, err := d.Resolve(ctx, types.SemanticSlackChannelID, []string{"C1"})
		gt.NoError(t, err).Required()
		gt.Equal(t, got, map[string]definition.Display{"C1": {}})
	})

	t.Run("unknown or empty semantic returns nothing", func(t *testing.T) {
		d := semantic.NewDisplayer(semantic.Deps{})
		for _, id := range []types.Semantic{"", "nope"} {
			got, err := d.Resolve(ctx, id, []string{"C1"})
			gt.NoError(t, err).Required()
			gt.Equal(t, got, map[string]definition.Display{})
		}
	})

	t.Run("lookup error is propagated", func(t *testing.T) {
		cause := errors.New("slack down")
		d := semantic.NewDisplayer(semantic.Deps{SlackChannelNames: &fakeChannelNames{err: cause}})
		_, err := d.Resolve(ctx, types.SemanticSlackChannelID, []string{"C1"})
		gt.Error(t, err).Is(cause)
	})
}
