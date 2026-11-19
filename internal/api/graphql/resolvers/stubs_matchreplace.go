package resolvers

// Contract stubs for matchreplace.graphqls (T-099). T-035 (CRUD, import/export) and T-102 (tester) replace this
// file with matchreplace.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// MatchReplaceService serves the Match & Replace operations. Placeholder until T-035 defines its methods.
type MatchReplaceService any

// MatchReplaceRule returns the MatchReplaceRule field resolver.
func (r *Resolver) MatchReplaceRule() generated.MatchReplaceRuleResolver { return &matchReplaceRuleResolver{r} }

type matchReplaceRuleResolver struct{ *Resolver }

func (r *queryResolver) MatchReplaceCollections(context.Context) ([]generated.MatchReplaceCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) MatchReplaceRule(context.Context, string) (*generated.MatchReplaceRule, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) ExportMatchReplace(context.Context) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateMatchReplaceCollection(
	context.Context, string,
) (*generated.MatchReplaceCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) RenameMatchReplaceCollection(
	context.Context, string, string,
) (*generated.MatchReplaceCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteMatchReplaceCollection(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) ReorderMatchReplaceCollections(
	context.Context, []string,
) ([]generated.MatchReplaceCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateMatchReplaceRule(
	context.Context, string, generated.MatchReplaceRuleInput,
) (*generated.MatchReplaceRule, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) UpdateMatchReplaceRule(
	context.Context, string, generated.MatchReplaceRuleInput,
) (*generated.MatchReplaceRule, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) SetMatchReplaceRuleEnabled(
	context.Context, string, bool,
) (*generated.MatchReplaceRule, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) MoveMatchReplaceRule(
	context.Context, string, string, int,
) (*generated.MatchReplaceRule, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteMatchReplaceRule(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) ImportMatchReplace(context.Context, string) (int, error) {
	return 0, apperr.ErrNotImplemented
}

func (r *mutationResolver) TestMatchReplace(
	context.Context, generated.MatchReplaceRuleInput, generated.MatchReplaceMessageKind, []byte,
) (*generated.MatchReplaceTestResult, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *matchReplaceRuleResolver) Collection(
	context.Context, *generated.MatchReplaceRule,
) (*generated.MatchReplaceCollection, error) {
	return nil, apperr.ErrNotImplemented
}
