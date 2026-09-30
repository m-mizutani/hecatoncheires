package semantic

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/slackchannelid"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Deps carries the external lookups that semantic resolvers need. A nil
// field means that service is not configured; every value of that semantic
// is then reported as unresolved.
type Deps struct {
	SlackChannelNames slackchannelid.ChannelNameLookup
}

// Displayer builds the supplementary Display for text values that carry a
// semantic. It holds only the resolvers registered at construction and no
// per-request state.
type Displayer struct {
	resolvers map[types.Semantic]definition.Resolver
}

// NewDisplayer registers the resolver of every semantic whose dependency is
// set in deps.
func NewDisplayer(deps Deps) *Displayer {
	resolvers := make(map[types.Semantic]definition.Resolver)
	if deps.SlackChannelNames != nil {
		resolvers[types.SemanticSlackChannelID] = slackchannelid.NewResolver(deps.SlackChannelNames)
	}
	return &Displayer{resolvers: resolvers}
}

// Resolve returns a Display for every non-empty value, keyed by value. A value
// whose name was found carries its Label and URL (Semantic.Link); any other
// value — the service is not configured, the lookup found nothing, or the
// value does not fit the semantic — gets an unresolved (empty) Display.
// Duplicates are removed before the resolver is called, and values that fail
// validation never reach it. An empty or unknown id returns an empty map and
// a nil error.
func (d *Displayer) Resolve(ctx context.Context, id types.Semantic, values []string) (map[string]definition.Display, error) {
	out := make(map[string]definition.Display)
	sem, ok := Lookup(id)
	if !ok {
		return out, nil
	}

	valid := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, dup := out[v]; dup {
			continue
		}
		out[v] = definition.Display{}
		if err := sem.Validate(v); err != nil {
			continue
		}
		valid = append(valid, v)
	}

	r, ok := d.resolvers[id]
	if !ok || len(valid) == 0 {
		return out, nil
	}
	labels, err := r.Resolve(ctx, valid)
	if err != nil {
		return nil, err
	}
	for _, v := range valid {
		if label := labels[v]; label != "" {
			out[v] = definition.Display{Label: label, URL: sem.Link(v)}
		}
	}
	return out, nil
}
