package resolvers

import (
	"context"
	"strconv"
	"strings"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/history"
)

// HistoryService is what the history resolvers need from [history.Service].
type HistoryService interface {
	List(ctx context.Context, a history.PageArgs) (history.Page, error)
	Get(ctx context.Context, id int64) (history.Request, error)
	EditedBy(ctx context.Context, id int64) (history.Request, bool, error)
	Head(ctx context.Context, id int64, side history.Part) ([]byte, error)
	Body(ctx context.Context, id int64, side history.Part, offset int64, limit int, decode bool) ([]byte, error)
	SetColor(ctx context.Context, ids []int64, color string) ([]history.Request, error)
	SetNote(ctx context.Context, id int64, note string) (history.Request, error)
	Delete(ctx context.Context, a history.DeleteArgs) (int64, error)
	Complete(ctx context.Context, lang history.Language, text string, cursor int) (history.Completions, error)
	SubscribeAdded(ctx context.Context, query, scopeID *string) (<-chan history.Request, error)
	SubscribeUpdated(ctx context.Context, query, scopeID *string) (<-chan history.Update, error)
	SubscribeDeleted(ctx context.Context) (<-chan []int64, error)
}

var _ HistoryService = (*history.Service)(nil)

// history is the configured service; without one the history operations
// report NOT_IMPLEMENTED.
func (r *Resolver) history() (HistoryService, error) {
	if r.deps.History == nil {
		return nil, apperr.ErrNotImplemented
	}

	return r.deps.History, nil
}

// parseRowID maps a GraphQL ID to a request row ID.
func parseRowID(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, apperr.New(apperr.NotFound, "request %q not found", id)
	}

	return n, nil
}

func parseRowIDs(ids []string) ([]int64, error) {
	out := make([]int64, len(ids))

	for i, id := range ids {
		n, err := parseRowID(id)
		if err != nil {
			return nil, err
		}

		out[i] = n
	}

	return out, nil
}

func formatIDs(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.FormatInt(id, 10)
	}

	return out
}

func toHTTPRequest(r history.Request) *generated.HTTPRequest {
	out := &generated.HTTPRequest{
		ID:          strconv.FormatInt(r.ID, 10),
		Source:      generated.RequestSource(strings.ToUpper(r.Source)),
		Host:        r.Host,
		Port:        r.Port,
		TLS:         r.TLS,
		Method:      r.Method,
		Path:        r.Path,
		Query:       r.Query,
		Ext:         r.Ext,
		Length:      r.Length,
		CreatedAt:   r.CreatedAt,
		IsDropped:   r.Dropped,
		IsTruncated: r.Truncated,
		BodyLength:  r.BodyLength,
	}

	if r.Color != "" {
		out.Color = new(generated.RequestColor(r.Color))
	}

	if r.Note != "" {
		out.Note = new(r.Note)
	}

	if r.EditedFrom != nil {
		// Only the ID is known here; the editedFrom field resolver loads the rest.
		out.EditedFrom = &generated.HTTPRequest{ID: strconv.FormatInt(*r.EditedFrom, 10)}
	}

	if p := r.Resp; p != nil {
		out.Response = &generated.HTTPResponse{
			Code:        p.Code,
			Length:      p.Length,
			RoundtripMs: int(p.RoundTripMs),
			CreatedAt:   p.CreatedAt,
			BodyLength:  p.BodyLength,
			RequestID:   r.ID,
		}
	}

	return out
}

func toHTTPRequests(rs []history.Request) []generated.HTTPRequest {
	out := make([]generated.HTTPRequest, len(rs))
	for i, r := range rs {
		out[i] = *toHTTPRequest(r)
	}

	return out
}

func (r *queryResolver) Requests(ctx context.Context, first *int, after *string, last *int, before *string, query, scopeID *string, sort *generated.RequestSort) (*generated.HTTPRequestConnection, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	a := history.PageArgs{First: first, After: after, Last: last, Before: before, Query: query, ScopeID: scopeID}
	if sort != nil {
		a.Sort = history.Sort{Field: history.SortField(sort.Field), Desc: sort.Direction == generated.SortDirectionDesc}
	}

	p, err := svc.List(ctx, a)
	if err != nil {
		return nil, err
	}

	conn := &generated.HTTPRequestConnection{
		Edges:      make([]generated.HTTPRequestEdge, len(p.Items)),
		TotalCount: p.Total,
		PageInfo:   &generated.PageInfo{HasNextPage: p.HasNext, HasPreviousPage: p.HasPrevious},
	}

	for i, it := range p.Items {
		conn.Edges[i] = generated.HTTPRequestEdge{Cursor: it.Cursor, Node: toHTTPRequest(it.Request)}
	}

	if n := len(p.Items); n > 0 {
		conn.PageInfo.StartCursor = new(p.Items[0].Cursor)
		conn.PageInfo.EndCursor = new(p.Items[n-1].Cursor)
	}

	return conn, nil
}

func (r *queryResolver) Request(ctx context.Context, id string) (*generated.HTTPRequest, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	n, perr := parseRowID(id)
	if perr != nil {
		return nil, nil //nolint:nilnil,nilerr // T-016: schema says null for an ID that names no request.
	}

	req, err := svc.Get(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil //nolint:nilnil // T-016: schema says null for an unknown ID.
	}

	if err != nil {
		return nil, err
	}

	return toHTTPRequest(req), nil
}

func (r *queryResolver) QueryCompletions(ctx context.Context, lang generated.QueryLanguage, text string, cursor int) (*generated.QueryCompletions, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	c, err := svc.Complete(ctx, history.Language(lang), text, cursor)
	if err != nil {
		return nil, err
	}

	out := &generated.QueryCompletions{From: c.From, To: c.To, Items: make([]generated.Completion, len(c.Items))}
	for i, it := range c.Items {
		out.Items[i] = generated.Completion{
			Label: it.Label, InsertText: it.InsertText, Kind: generated.CompletionKind(it.Kind),
			Detail: it.Detail, IsExtension: it.IsExtension,
		}
	}

	return out, nil
}

func (r *mutationResolver) SetRequestColor(ctx context.Context, ids []string, color *generated.RequestColor) ([]generated.HTTPRequest, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	rows, err := parseRowIDs(ids)
	if err != nil {
		return nil, err
	}

	var c string
	if color != nil {
		c = string(*color)
	}

	rs, err := svc.SetColor(ctx, rows, c)
	if err != nil {
		return nil, err
	}

	return toHTTPRequests(rs), nil
}

func (r *mutationResolver) SetRequestNote(ctx context.Context, id string, note *string) (*generated.HTTPRequest, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	n, err := parseRowID(id)
	if err != nil {
		return nil, err
	}

	req, err := svc.SetNote(ctx, n, deref(note))
	if err != nil {
		return nil, err
	}

	return toHTTPRequest(req), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func (r *mutationResolver) DeleteRequests(ctx context.Context, ids []string, query, scopeID *string) (int64, error) {
	svc, err := r.history()
	if err != nil {
		return 0, err
	}

	a := history.DeleteArgs{Query: query, ScopeID: scopeID}

	if ids != nil {
		// Unknown IDs are ignored, so an ID that cannot name a row is dropped.
		a.IDs = []int64{}

		for _, id := range ids {
			if n, err := parseRowID(id); err == nil {
				a.IDs = append(a.IDs, n)
			}
		}
	}

	return svc.Delete(ctx, a)
}

// forward maps each value of in to the result channel until ctx ends or in
// closes (the service closes it when ctx ends or the subscriber lagged).
func forward[In, Out any](ctx context.Context, in <-chan In, conv func(In) Out) <-chan Out {
	out := make(chan Out)

	go func() {
		defer close(out)

		for v := range in {
			select {
			case out <- conv(v):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (r *subscriptionResolver) RequestAdded(ctx context.Context, query, scopeID *string) (<-chan *generated.HTTPRequest, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	ch, err := svc.SubscribeAdded(ctx, query, scopeID)
	if err != nil {
		return nil, err
	}

	return forward(ctx, ch, func(q history.Request) *generated.HTTPRequest { return toHTTPRequest(q) }), nil
}

func (r *subscriptionResolver) RequestUpdated(ctx context.Context, query, scopeID *string) (<-chan *generated.RequestUpdate, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	ch, err := svc.SubscribeUpdated(ctx, query, scopeID)
	if err != nil {
		return nil, err
	}

	return forward(ctx, ch, func(u history.Update) *generated.RequestUpdate {
		return &generated.RequestUpdate{Request: toHTTPRequest(u.Request), Matches: u.Matches}
	}), nil
}

func (r *subscriptionResolver) RequestsDeleted(ctx context.Context) (<-chan *generated.RequestsDeleted, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	ch, err := svc.SubscribeDeleted(ctx)
	if err != nil {
		return nil, err
	}

	return forward(ctx, ch, func(ids []int64) *generated.RequestsDeleted {
		return &generated.RequestsDeleted{Ids: formatIDs(ids)}
	}), nil
}

// Resolver factories for the lazily loaded HttpRequest/HttpResponse fields.

// HttpRequest returns the HttpRequest field resolver.
func (r *Resolver) HttpRequest() generated.HttpRequestResolver { return &httpRequestResolver{r} } //nolint:revive // T-013: name fixed by gqlgen ResolverRoot (GraphQL type HttpRequest).

// HttpResponse returns the HttpResponse field resolver.
func (r *Resolver) HttpResponse() generated.HttpResponseResolver { return &httpResponseResolver{r} } //nolint:revive // T-013: name fixed by gqlgen ResolverRoot (GraphQL type HttpResponse).

type (
	httpRequestResolver  struct{ *Resolver }
	httpResponseResolver struct{ *Resolver }
)

func (r *httpRequestResolver) EditedFrom(ctx context.Context, obj *generated.HTTPRequest) (*generated.HTTPRequest, error) {
	if obj.EditedFrom == nil {
		return nil, nil //nolint:nilnil // T-016: null for unedited records.
	}

	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	n, err := parseRowID(obj.EditedFrom.ID)
	if err != nil {
		return nil, err
	}

	orig, err := svc.Get(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil //nolint:nilnil // T-016: the original was deleted.
	}

	if err != nil {
		return nil, err
	}

	return toHTTPRequest(orig), nil
}

func (r *httpRequestResolver) EditedBy(ctx context.Context, obj *generated.HTTPRequest) (*generated.HTTPRequest, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	id, err := parseRowID(obj.ID)
	if err != nil {
		return nil, err
	}

	edited, found, err := svc.EditedBy(ctx, id)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil //nolint:nilnil // T-100: null when there is no edited record.
	}

	return toHTTPRequest(edited), nil
}

func (r *httpRequestResolver) Head(ctx context.Context, obj *generated.HTTPRequest) ([]byte, error) {
	n, err := parseRowID(obj.ID)
	if err != nil {
		return nil, err
	}

	return r.head(ctx, n, history.PartRequest)
}

func (r *httpRequestResolver) Body(ctx context.Context, obj *generated.HTTPRequest, offset int64, limit int, decode bool) ([]byte, error) {
	n, err := parseRowID(obj.ID)
	if err != nil {
		return nil, err
	}

	return r.body(ctx, n, history.PartRequest, offset, limit, decode)
}

func (r *httpResponseResolver) Head(ctx context.Context, obj *generated.HTTPResponse) ([]byte, error) {
	return r.head(ctx, obj.RequestID, history.PartResponse)
}

func (r *httpResponseResolver) Body(ctx context.Context, obj *generated.HTTPResponse, offset int64, limit int, decode bool) ([]byte, error) {
	return r.body(ctx, obj.RequestID, history.PartResponse, offset, limit, decode)
}

func (r *Resolver) head(ctx context.Context, id int64, side history.Part) ([]byte, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	return svc.Head(ctx, id, side)
}

func (r *Resolver) body(ctx context.Context, id int64, side history.Part, offset int64, limit int, decode bool) ([]byte, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}

	return svc.Body(ctx, id, side, offset, limit, decode)
}
