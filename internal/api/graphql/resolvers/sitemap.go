package resolvers

// Contract stub for sitemap.graphqls (T-094); T-020 replaces it with the real resolver.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// SitemapNodes returns the children of parentID, or the host roots.
func (q *queryResolver) SitemapNodes(
	_ context.Context, _, _, _ *string, _ bool,
) ([]generated.SitemapNode, error) {
	return nil, apperr.ErrNotImplemented
}
