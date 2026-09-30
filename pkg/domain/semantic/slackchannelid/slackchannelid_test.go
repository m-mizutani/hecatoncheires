package slackchannelid_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/slackchannelid"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestValidate(t *testing.T) {
	s := slackchannelid.New()

	for _, v := range []string{"C0123ABCD", "C1", "G0123ABCD"} {
		t.Run("accepts "+v, func(t *testing.T) {
			gt.NoError(t, s.Validate(v))
		})
	}

	for _, v := range []string{
		"D0123", "U0123ABCD", "c0123abcd", "g0123abcd", "C", "G", "#general",
		"<#C0123ABCD>", " C0123ABCD", "C0123-ABC",
	} {
		t.Run("rejects "+v, func(t *testing.T) {
			gt.Error(t, s.Validate(v)).Is(definition.ErrInvalidValue)
		})
	}
}

func TestRendering(t *testing.T) {
	s := slackchannelid.New()
	gt.Value(t, s.ID()).Equal(types.SemanticSlackChannelID)
	gt.Value(t, s.SlackMrkdwn("C0123ABCD")).Equal("<#C0123ABCD>")
	gt.Value(t, s.Link("C0123ABCD")).Equal("https://slack.com/archives/C0123ABCD")
	gt.True(t, strings.Contains(s.PromptHint(), "C0123456789"))
}

type fakeLookup struct {
	names map[string]string
	err   error
	calls [][]string
}

func (f *fakeLookup) GetChannelNames(_ context.Context, ids []string) (map[string]string, error) {
	f.calls = append(f.calls, ids)
	if f.err != nil {
		return nil, f.err
	}
	return f.names, nil
}

func TestResolver(t *testing.T) {
	t.Run("returns # + name for resolved ids only", func(t *testing.T) {
		lookup := &fakeLookup{names: map[string]string{"C1": "general"}}
		got, err := slackchannelid.NewResolver(lookup).Resolve(context.Background(), []string{"C1", "C2"})
		gt.NoError(t, err).Required()
		gt.Equal(t, got, map[string]string{"C1": "#general"})
		gt.Equal(t, lookup.calls, [][]string{{"C1", "C2"}})
	})

	t.Run("propagates the lookup error", func(t *testing.T) {
		cause := errors.New("slack down")
		lookup := &fakeLookup{err: cause}
		_, err := slackchannelid.NewResolver(lookup).Resolve(context.Background(), []string{"C1"})
		gt.Error(t, err).Is(cause)
	})
}
