package resolvers

import (
	"context"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/project"
)

// ProjectService is what the project resolvers need from [project.Service].
type ProjectService interface {
	List(ctx context.Context) ([]project.Project, error)
	Get(ctx context.Context, id int64) (project.Project, error)
	Current(ctx context.Context) (project.Project, error)
	Create(ctx context.Context, name string, sel bool) (project.Project, error)
	Rename(ctx context.Context, id int64, name string) (project.Project, error)
	Duplicate(ctx context.Context, id int64, name string) (project.Project, error)
	Delete(ctx context.Context, id int64) error
	SetDefault(ctx context.Context, id *int64) (*project.Project, error)
	Select(ctx context.Context, id int64) (project.Project, error)
	Subscribe(ctx context.Context) <-chan project.Change
}

// projects is the configured service; without one (tests, early wiring) the
// project operations report NOT_IMPLEMENTED.
func (r *Resolver) projects() (ProjectService, error) {
	if r.deps.Projects == nil {
		return nil, apperr.ErrNotImplemented
	}

	return r.deps.Projects, nil
}

// parseProjectID maps a GraphQL ID to a row ID; an ID that is not a number
// cannot name a project.
func parseProjectID(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, apperr.New(apperr.NotFound, "project %q not found", id)
	}

	return n, nil
}

func toProject(p project.Project) *generated.Project {
	return &generated.Project{
		ID:           strconv.FormatInt(p.ID, 10),
		Name:         p.Name,
		IsTemporary:  p.Temporary,
		IsDefault:    p.Default,
		IsSelected:   p.Selected,
		CreatedAt:    p.CreatedAt,
		LastOpenedAt: p.LastOpenedAt,
		DbSizeBytes:  p.DBSize,
	}
}

func (r *queryResolver) Projects(ctx context.Context) ([]generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	ps, err := svc.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]generated.Project, len(ps))
	for i, p := range ps {
		out[i] = *toProject(p)
	}

	return out, nil
}

func (r *queryResolver) Project(ctx context.Context, id string) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	n, err := parseProjectID(id)
	if err != nil {
		return nil, nil //nolint:nilerr // T-006: an ID that is no number names no project: null, not an error
	}

	p, err := svc.Get(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil // documented: null if no project has this ID
	}

	if err != nil {
		return nil, err
	}

	return toProject(p), nil
}

func (r *queryResolver) CurrentProject(ctx context.Context) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	p, err := svc.Current(ctx)
	if err != nil {
		return nil, err
	}

	return toProject(p), nil
}

func (r *mutationResolver) CreateProject(ctx context.Context, name string, sel bool) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	p, err := svc.Create(ctx, name, sel)
	if err != nil {
		return nil, err
	}

	return toProject(p), nil
}

func (r *mutationResolver) RenameProject(ctx context.Context, id, name string) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	n, err := parseProjectID(id)
	if err != nil {
		return nil, err
	}

	p, err := svc.Rename(ctx, n, name)
	if err != nil {
		return nil, err
	}

	return toProject(p), nil
}

func (r *mutationResolver) DuplicateProject(ctx context.Context, id string, name *string) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	n, err := parseProjectID(id)
	if err != nil {
		return nil, err
	}

	var newName string
	if name != nil {
		newName = *name
	}

	p, err := svc.Duplicate(ctx, n, newName)
	if err != nil {
		return nil, err
	}

	return toProject(p), nil
}

func (r *mutationResolver) DeleteProject(ctx context.Context, id string) (string, error) {
	svc, err := r.projects()
	if err != nil {
		return "", err
	}

	n, err := parseProjectID(id)
	if err != nil {
		return "", err
	}

	if err := svc.Delete(ctx, n); err != nil {
		return "", err
	}

	return strconv.FormatInt(n, 10), nil
}

func (r *mutationResolver) SetDefaultProject(ctx context.Context, id *string) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	var n *int64

	if id != nil {
		v, err := parseProjectID(*id)
		if err != nil {
			return nil, err
		}

		n = &v
	}

	p, err := svc.SetDefault(ctx, n)
	if err != nil || p == nil {
		return nil, err
	}

	return toProject(*p), nil
}

func (r *mutationResolver) SelectProject(ctx context.Context, id string) (*generated.Project, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	n, err := parseProjectID(id)
	if err != nil {
		return nil, err
	}

	p, err := svc.Select(ctx, n)
	if err != nil {
		return nil, err
	}

	return toProject(p), nil
}

// ProjectChanged forwards the service's events until ctx ends or the service
// closes the subscription (shutdown, or the client fell too far behind and must
// resubscribe).
func (r *subscriptionResolver) ProjectChanged(ctx context.Context) (<-chan *generated.ProjectChange, error) {
	svc, err := r.projects()
	if err != nil {
		return nil, err
	}

	src := svc.Subscribe(ctx)
	out := make(chan *generated.ProjectChange)

	go func() {
		defer close(out)

		for c := range src {
			ev := &generated.ProjectChange{
				Kind:      generated.ProjectChangeKind(c.Kind),
				ProjectID: strconv.FormatInt(c.ProjectID, 10),
			}
			if c.Project != nil {
				ev.Project = toProject(*c.Project)
			}

			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}
