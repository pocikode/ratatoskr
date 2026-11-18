package resolvers_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/project"
	"github.com/pocikode/ratatoskr/internal/store"
)

const projectFields = `id name isTemporary isDefault isSelected createdAt lastOpenedAt dbSizeBytes`

type projectJSON struct {
	ID           string
	Name         string
	IsTemporary  bool
	IsDefault    bool
	IsSelected   bool
	CreatedAt    string
	LastOpenedAt *string
	DbSizeBytes  string
}

// newClient serves the real handler over a fresh data dir.
func newClient(t *testing.T) (*client.Client, *project.Service) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { root.Close() })

	inst, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { inst.Close() })

	svc, err := project.Open(t.Context(), project.Deps{Instance: inst, Root: root})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { svc.Close() })

	h := graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(resolvers.Deps{Projects: svc})})

	return client.New(h), svc
}

func gqlErrCode(err error) string {
	// The gqlgen client reports GraphQL errors as a formatted list that
	// includes the extensions.
	if err == nil {
		return ""
	}

	for _, code := range []string{"NOT_FOUND", "BAD_INPUT", "NOT_IMPLEMENTED", "INTERNAL"} {
		if strings.Contains(err.Error(), `"`+code+`"`) {
			return code
		}
	}

	return err.Error()
}

func TestProjectOperations(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)

	var cur struct{ CurrentProject projectJSON }
	if err := c.Post(`{ currentProject { `+projectFields+` } }`, &cur); err != nil {
		t.Fatal(err)
	}

	if !cur.CurrentProject.IsTemporary || !cur.CurrentProject.IsSelected {
		t.Fatalf("currentProject = %+v", cur.CurrentProject)
	}

	var created struct{ CreateProject projectJSON }
	if err := c.Post(`mutation { createProject(name: " Alpha ") { `+projectFields+` } }`, &created); err != nil {
		t.Fatal(err)
	}

	a := created.CreateProject
	if a.Name != "Alpha" || a.IsSelected || a.IsTemporary {
		t.Fatalf("created = %+v", a)
	}

	var renamed struct{ RenameProject projectJSON }
	if err := c.Post(`mutation($id: ID!) { renameProject(id: $id, name: "Beta") { name } }`, &renamed, client.Var("id", a.ID)); err != nil {
		t.Fatal(err)
	}

	if renamed.RenameProject.Name != "Beta" {
		t.Errorf("renamed = %+v", renamed)
	}

	var dup struct{ DuplicateProject projectJSON }
	if err := c.Post(`mutation($id: ID!) { duplicateProject(id: $id) { id name isSelected } }`, &dup, client.Var("id", a.ID)); err != nil {
		t.Fatal(err)
	}

	if dup.DuplicateProject.Name != "Beta (copy)" || dup.DuplicateProject.IsSelected {
		t.Errorf("duplicate = %+v", dup)
	}

	var def struct{ SetDefaultProject *projectJSON }
	if err := c.Post(`mutation($id: ID) { setDefaultProject(id: $id) { id isDefault } }`, &def, client.Var("id", a.ID)); err != nil {
		t.Fatal(err)
	}

	if def.SetDefaultProject == nil || !def.SetDefaultProject.IsDefault {
		t.Errorf("setDefault = %+v", def)
	}

	var sel struct{ SelectProject projectJSON }
	if err := c.Post(`mutation($id: ID!) { selectProject(id: $id) { id isSelected isDefault } }`, &sel, client.Var("id", a.ID)); err != nil {
		t.Fatal(err)
	}

	if !sel.SelectProject.IsSelected || !sel.SelectProject.IsDefault {
		t.Errorf("select = %+v", sel)
	}

	var list struct{ Projects []projectJSON }
	if err := c.Post(`{ projects { id isSelected } }`, &list); err != nil {
		t.Fatal(err)
	}

	if len(list.Projects) != 3 || list.Projects[0].ID != a.ID {
		t.Errorf("projects = %+v, want 3 with the selected one first", list.Projects)
	}

	var one struct{ Project *projectJSON }
	if err := c.Post(`{ project(id: "999999") { id } }`, &one); err != nil || one.Project != nil {
		t.Errorf("unknown project = %+v, %v", one.Project, err)
	}

	if err := c.Post(`{ project(id: "nope") { id } }`, &one); err != nil || one.Project != nil {
		t.Errorf("malformed id = %+v, %v", one.Project, err)
	}

	var clr struct{ SetDefaultProject *projectJSON }
	if err := c.Post(`mutation { setDefaultProject(id: null) { id } }`, &clr); err != nil || clr.SetDefaultProject != nil {
		t.Errorf("clear default = %+v, %v", clr, err)
	}

	var del struct{ DeleteProject string }
	if err := c.Post(`mutation($id: ID!) { deleteProject(id: $id) }`, &del, client.Var("id", a.ID)); err != nil {
		t.Fatal(err)
	}

	if del.DeleteProject != a.ID {
		t.Errorf("deleteProject = %q, want %q", del.DeleteProject, a.ID)
	}

	if err := c.Post(`{ currentProject { id isSelected } }`, &cur); err != nil || cur.CurrentProject.ID == a.ID {
		t.Errorf("current after deleting selected = %+v, %v", cur.CurrentProject, err)
	}
}

func TestProjectErrors(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)

	tests := []struct {
		name, query, code string
	}{
		{"create empty", `mutation { createProject(name: "  ") { id } }`, "BAD_INPUT"},
		{"rename blank", `mutation { renameProject(id: "1", name: "") { id } }`, "BAD_INPUT"},
		{"rename unknown", `mutation { renameProject(id: "999", name: "x") { id } }`, "NOT_FOUND"},
		{"rename malformed id", `mutation { renameProject(id: "abc", name: "x") { id } }`, "NOT_FOUND"},
		{"duplicate unknown", `mutation { duplicateProject(id: "999") { id } }`, "NOT_FOUND"},
		{"delete unknown", `mutation { deleteProject(id: "999") }`, "NOT_FOUND"},
		{"select unknown", `mutation { selectProject(id: "999") { id } }`, "NOT_FOUND"},
		{"default unknown", `mutation { setDefaultProject(id: "999") { id } }`, "NOT_FOUND"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out map[string]any

			if got := gqlErrCode(c.Post(tt.query, &out)); got != tt.code {
				t.Errorf("code = %s, want %s", got, tt.code)
			}
		})
	}

	// The selected project is temporary: it cannot be the default.
	var cur struct{ CurrentProject projectJSON }
	if err := c.Post(`{ currentProject { id } }`, &cur); err != nil {
		t.Fatal(err)
	}

	var out map[string]any

	err := c.Post(`mutation($id: ID) { setDefaultProject(id: $id) { id } }`, &out, client.Var("id", cur.CurrentProject.ID))
	if got := gqlErrCode(err); got != "BAD_INPUT" {
		t.Errorf("default on temp = %s", got)
	}
}

func TestProjectChangedSubscription(t *testing.T) {
	t.Parallel()

	_, svc := newClient(t)
	r := resolvers.New(resolvers.Deps{Projects: svc})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch, err := r.Subscription().ProjectChanged(ctx)
	if err != nil {
		t.Fatal(err)
	}

	p, err := svc.Create(t.Context(), "x", true)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"CREATED", "SELECTED"} {
		ev := <-ch
		if string(ev.Kind) != want || ev.ProjectID != itoa(p.ID) || ev.Project == nil {
			t.Fatalf("event = %+v, want %s", ev, want)
		}
	}

	if err := svc.Delete(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}

	for ev := range ch {
		if string(ev.Kind) == "DELETED" {
			if ev.Project != nil || ev.ProjectID != itoa(p.ID) {
				t.Errorf("DELETED event = %+v", ev)
			}

			break
		}
	}

	cancel()

	for range ch { // drains until closed
	}
}

func TestProjectsNotConfigured(t *testing.T) {
	t.Parallel()

	r := resolvers.New(resolvers.Deps{})

	if _, err := r.Query().Projects(t.Context()); err == nil {
		t.Error("Projects without service succeeded")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
