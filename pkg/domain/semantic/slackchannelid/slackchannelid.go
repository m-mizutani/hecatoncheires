// Package slackchannelid implements the slack_channel_id semantic: a text
// value that is the ID of a Slack channel.
package slackchannelid

import (
	"context"
	"regexp"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Slack documents three conversation ID prefixes: C (public channels and
// private channels created since March 2021), G (older private channels and
// multi-person DMs) and D (direct messages). C and G are accepted; D is not a
// channel. The length is not documented, so only the prefix and the character
// set are checked.
// https://docs.slack.dev/apis/web-api/using-the-conversations-api/
var pattern = regexp.MustCompile(`^[CG][A-Z0-9]+$`)

type slackChannelID struct{}

// New returns the slack_channel_id semantic.
func New() definition.Semantic { return slackChannelID{} }

func (slackChannelID) ID() types.Semantic { return types.SemanticSlackChannelID }

func (slackChannelID) Validate(value string) error {
	if !pattern.MatchString(value) {
		return goerr.Wrap(definition.ErrInvalidValue,
			`Slack channel ID must be "C" or "G" followed by uppercase letters and digits (e.g. C0123456789), not a channel name, a <#...> link or a direct message ID ("D...")`,
			goerr.V("semantic", types.SemanticSlackChannelID),
			goerr.V("value", value))
	}
	return nil
}

func (slackChannelID) PromptHint() string {
	return `Slack channel ID: "C" or "G" followed by uppercase letters and digits (e.g. C0123456789). ` +
		`Write the ID itself, not a channel name ("#general") and not a mention ("<#C0123456789>").`
}

func (slackChannelID) SlackMrkdwn(value string) string { return "<#" + value + ">" }

// Link uses the workspace-independent archive URL. The workspace URL would
// need auth.test, whose failure the Slack client caches for the life of the
// process, and the Case list query carries this link.
func (slackChannelID) Link(value string) string { return "https://slack.com/archives/" + value }

// ChannelNameLookup is the one method this semantic needs from Slack.
// pkg/service/slack.Service satisfies it; this package never imports it, so
// the domain layer does not depend on the service layer.
type ChannelNameLookup interface {
	GetChannelNames(ctx context.Context, ids []string) (map[string]string, error)
}

type resolver struct {
	lookup ChannelNameLookup
}

// NewResolver returns the channel-name resolver. lookup must be non-nil.
func NewResolver(lookup ChannelNameLookup) definition.Resolver {
	return resolver{lookup: lookup}
}

// Resolve calls GetChannelNames once and returns "#" + name per resolved id.
func (r resolver) Resolve(ctx context.Context, values []string) (map[string]string, error) {
	names, err := r.lookup.GetChannelNames(ctx, values)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get Slack channel names",
			goerr.V("channel_count", len(values)))
	}
	out := make(map[string]string, len(names))
	for _, id := range values {
		if name, ok := names[id]; ok && name != "" {
			out[id] = "#" + name
		}
	}
	return out, nil
}
