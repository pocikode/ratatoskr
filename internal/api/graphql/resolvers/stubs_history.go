package resolvers

// Contract stubs for history.graphqls (T-013). The history task replaces them
// with history.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// HttpRequest returns the HttpRequest field resolver.
func (r *Resolver) HttpRequest() generated.HttpRequestResolver { return &httpRequestResolver{r} }

// HttpResponse returns the HttpResponse field resolver.
func (r *Resolver) HttpResponse() generated.HttpResponseResolver { return &httpResponseResolver{r} }

type (
	httpRequestResolver  struct{ *Resolver }
	httpResponseResolver struct{ *Resolver }
)

func (r *queryResolver) Requests(context.Context, *int, *string, *int, *string, *string, *string, *generated.RequestSort) (*generated.HTTPRequestConnection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) Request(context.Context, string) (*generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) QueryCompletions(context.Context, generated.QueryLanguage, string, int) (*generated.QueryCompletions, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) SetRequestColor(context.Context, []string, *generated.RequestColor) ([]generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) SetRequestNote(context.Context, string, *string) (*generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteRequests(context.Context, []string, *string, *string) (int64, error) {
	return 0, apperr.ErrNotImplemented
}

func (r *subscriptionResolver) RequestAdded(context.Context, *string, *string) (<-chan *generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *subscriptionResolver) RequestUpdated(context.Context, *string, *string) (<-chan *generated.RequestUpdate, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *subscriptionResolver) RequestsDeleted(context.Context) (<-chan *generated.RequestsDeleted, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *httpRequestResolver) EditedFrom(context.Context, *generated.HTTPRequest) (*generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *httpRequestResolver) Head(context.Context, *generated.HTTPRequest) ([]byte, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *httpRequestResolver) Body(context.Context, *generated.HTTPRequest, int64, int, bool) ([]byte, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *httpResponseResolver) Head(context.Context, *generated.HTTPResponse) ([]byte, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *httpResponseResolver) Body(context.Context, *generated.HTTPResponse, int64, int, bool) ([]byte, error) {
	return nil, apperr.ErrNotImplemented
}
