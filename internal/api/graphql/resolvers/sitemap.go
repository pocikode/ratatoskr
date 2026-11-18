package resolvers

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/history"
)

type sitemapService interface {
	SitemapNodes(context.Context, *string, *string, *string, bool) ([]history.SitemapNode, error)
}

// SitemapNodes returns the children of parentID, or the host roots.
func (q *queryResolver) SitemapNodes(
	ctx context.Context, parentID, query, scopeID *string, recursive bool,
) ([]generated.SitemapNode, error) {
	svc, ok := q.deps.History.(sitemapService)
	if !ok {
		return nil, apperr.ErrNotImplemented
	}
	nodes, err := svc.SitemapNodes(ctx, parentID, query, scopeID, recursive)
	if err != nil {
		return nil, err
	}
	out := make([]generated.SitemapNode, len(nodes))
	for i, n := range nodes {
		out[i] = generated.SitemapNode{
			ID: n.ID, ParentID: n.ParentID, Kind: generated.SitemapNodeKind(n.Kind),
			Label: n.Label, URL: n.URL, Host: n.Host, Port: n.Port, TLS: n.TLS,
			RequestCount: n.RequestCount, HasChildren: n.HasChildren, Query: n.Query,
		}
	}
	return out, nil
}
