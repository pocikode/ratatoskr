package resolvers_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/environment"
	"github.com/pocikode/ratatoskr/internal/secret"
	"github.com/pocikode/ratatoskr/internal/store"
)

func environmentClient(t *testing.T, wired bool) *client.Client {
	t.Helper()
	deps := resolvers.Deps{}
	if wired {
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = root.Close() })
		inst, err := store.OpenInstance(t.Context(), root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = inst.Close() })
		proj, err := store.OpenProject(t.Context(), root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = proj.Close() })
		box, _, err := secret.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		deps.Environment = environment.New(environment.Deps{Instance: inst, Project: func() *store.DB { return proj }, Secrets: box})
	}
	return client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(deps)}))
}

type envResult struct {
	ID, Name           string
	IsGlobal, Selected bool
	Variables          []struct {
		Name, Value string
		Secret      bool
	}
}

func TestEnvironmentResolvers(t *testing.T) {
	t.Parallel()
	c := environmentClient(t, true)
	var list struct{ Environments []envResult }
	if err := c.Post(`{ environments { id name isGlobal selected variables { name value secret } } }`, &list); err != nil || len(list.Environments) != 1 || !list.Environments[0].IsGlobal {
		t.Fatalf("initial list = %+v, %v", list, err)
	}
	global := list.Environments[0].ID
	var created struct{ CreateEnvironment envResult }
	const create = `mutation($name: String!) { createEnvironment(name: $name) { id name selected variables { name } } }`
	if err := c.Post(create, &created, client.Var("name", " Dev ")); err != nil || created.CreateEnvironment.Name != "Dev" || created.CreateEnvironment.Selected {
		t.Fatalf("create = %+v, %v", created, err)
	}
	id := created.CreateEnvironment.ID
	wantCode(t, c.Post(create, &created, client.Var("name", "Dev")), "CONFLICT")
	wantCode(t, c.Post(create, &created, client.Var("name", "Global")), "CONFLICT")
	wantCode(t, c.Post(create, &created, client.Var("name", "")), "BAD_INPUT")
	var set struct{ SetEnvironmentVariables envResult }
	const setVars = `mutation($id: ID!, $vars: [EnvironmentVariableInput!]!) { setEnvironmentVariables(id: $id, variables: $vars) { id variables { name value secret } } }`
	vars := []map[string]any{{"name": "z", "value": "secret-value", "secret": true}, {"name": "a", "value": "plain"}}
	for _, target := range []string{global, id} {
		if err := c.Post(setVars, &set, client.Var("id", target), client.Var("vars", vars)); err != nil || len(set.SetEnvironmentVariables.Variables) != 2 || set.SetEnvironmentVariables.Variables[0].Value != "secret-value" || !set.SetEnvironmentVariables.Variables[0].Secret || set.SetEnvironmentVariables.Variables[1].Secret {
			t.Fatalf("set = %+v, %v", set, err)
		}
	}
	var one struct{ Environment *envResult }
	const query = `query($id: ID!) { environment(id: $id) { id name selected variables { name value secret } } }`
	if err := c.Post(query, &one, client.Var("id", id)); err != nil || one.Environment == nil || !reflect.DeepEqual(one.Environment.Variables, set.SetEnvironmentVariables.Variables) {
		t.Fatalf("get = %+v, %v", one, err)
	}
	wantCode(t, c.Post(setVars, &set, client.Var("id", id), client.Var("vars", []map[string]any{{"name": "bad name", "value": "v"}})), "BAD_INPUT")
	if err := c.Post(query, &one, client.Var("id", id)); err != nil || len(one.Environment.Variables) != 2 {
		t.Fatalf("invalid replacement changed list: %+v, %v", one, err)
	}
	for _, unknown := range []string{"999", "invalid", "0", "-1"} {
		one.Environment = nil
		if err := c.Post(query, &one, client.Var("id", unknown)); err != nil || one.Environment != nil {
			t.Fatalf("unknown %q = %+v, %v", unknown, one, err)
		}
	}
	var selected struct{ SelectEnvironment *envResult }
	const selectEnv = `mutation($id: ID) { selectEnvironment(id: $id) { id selected } }`
	if err := c.Post(selectEnv, &selected, client.Var("id", id)); err != nil || selected.SelectEnvironment == nil || !selected.SelectEnvironment.Selected {
		t.Fatalf("select = %+v, %v", selected, err)
	}
	wantCode(t, c.Post(selectEnv, &selected, client.Var("id", global)), "BAD_INPUT")
	wantCode(t, c.Post(selectEnv, &selected, client.Var("id", "999")), "NOT_FOUND")
	selected.SelectEnvironment = nil
	if err := c.Post(selectEnv, &selected, client.Var("id", nil)); err != nil || selected.SelectEnvironment != nil {
		t.Fatalf("deselect = %+v, %v", selected, err)
	}
	var rename struct{ RenameEnvironment envResult }
	const renameEnv = `mutation($id: ID!) { renameEnvironment(id: $id, name: "New") { id name } }`
	if err := c.Post(renameEnv, &rename, client.Var("id", id)); err != nil || rename.RenameEnvironment.Name != "New" {
		t.Fatalf("rename = %+v, %v", rename, err)
	}
	wantCode(t, c.Post(renameEnv, &rename, client.Var("id", global)), "CONFLICT")
	wantCode(t, c.Post(renameEnv, &rename, client.Var("id", "999")), "NOT_FOUND")
	var deleted struct{ DeleteEnvironment string }
	const deleteEnv = `mutation($id: ID!) { deleteEnvironment(id: $id) }`
	wantCode(t, c.Post(deleteEnv, &deleted, client.Var("id", global)), "CONFLICT")
	wantCode(t, c.Post(deleteEnv, &deleted, client.Var("id", "invalid")), "NOT_FOUND")
	if err := c.Post(selectEnv, &selected, client.Var("id", id)); err != nil {
		t.Fatal(err)
	}
	if err := c.Post(deleteEnv, &deleted, client.Var("id", id)); err != nil || deleted.DeleteEnvironment != id {
		t.Fatalf("delete = %+v, %v", deleted, err)
	}
	if err := c.Post(`{ environments { id isGlobal selected } }`, &list); err != nil || len(list.Environments) != 1 || list.Environments[0].Selected {
		t.Fatalf("after delete = %+v, %v", list, err)
	}
}

func TestEnvironmentResolversUnwired(t *testing.T) {
	t.Parallel()
	c := environmentClient(t, false)
	var out any
	for _, query := range []string{
		`{ environments { id } }`, `{ environment(id: "global") { id } }`,
		`mutation { createEnvironment(name: "Dev") { id } }`,
		`mutation { renameEnvironment(id: "global", name: "Dev") { id } }`,
		`mutation { deleteEnvironment(id: "global") }`,
		`mutation { setEnvironmentVariables(id: "global", variables: []) { id } }`,
		`mutation { selectEnvironment(id: null) { id } }`,
	} {
		wantCode(t, c.Post(query, &out), "NOT_IMPLEMENTED")
	}
}
