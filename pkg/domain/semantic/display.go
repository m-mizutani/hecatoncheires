package semantic

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/slackchannelid"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Deps carries the external lookups that semantic resolvers need. A nil
// field means that service is not configured; the semantic then resolves no
// label, while its link is still produced.
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

// Resolve returns displays keyed by value. For every value that passes
// validation, URL is Semantic.Link(value) and Label comes from the semantic's
// resolver when one is registered and found a label. A value whose Label and
// URL are both empty is absent. Duplicates are removed before the resolver is
// called, and invalid values never reach it. An empty or unknown id returns
// an empty map and a nil error.
func (d *Displayer) Resolve(ctx context.Context, id types.Semantic, values []string) (map[string]definition.Display, error) {
	out := make(map[string]definition.Display)
	sem, ok := Lookup(id)
	if !ok {
		return out, nil
	}

	seen := make(map[string]struct{}, len(values))
	valid := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		if err := sem.Validate(v); err != nil {
			continue
		}
		valid = append(valid, v)
	}
	if len(valid) == 0 {
		return out, nil
	}

	labels := map[string]string{}
	if r, ok := d.resolvers[id]; ok {
		resolved, err := r.Resolve(ctx, valid)
		if err != nil {
			return nil, err
		}
		labels = resolved
	}

	for _, v := range valid {
		disp := definition.Display{Label: labels[v], URL: sem.Link(v)}
		if disp.Label == "" && disp.URL == "" {
			continue
		}
		out[v] = disp
	}
	return out, nil
}
