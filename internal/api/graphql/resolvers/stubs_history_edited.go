package resolvers

// Contract stub for HttpRequest.editedBy (T-030). T-100 moves it next to EditedFrom in history.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func (r *httpRequestResolver) EditedBy(context.Context, *generated.HTTPRequest) (*generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}
