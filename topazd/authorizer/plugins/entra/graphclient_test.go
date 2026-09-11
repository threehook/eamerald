//nolint:testpackage
package entra

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	nethttplibrary "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/stretchr/testify/require"
)

// noopAuth satisfies kiota's AuthenticationProvider without adding any
// credentials to the request; the test server doesn't check for them.
type noopAuth struct{}

func (noopAuth) AuthenticateRequest(_ context.Context, _ *abstractions.RequestInformation, _ map[string]any) error {
	return nil
}

// newTestGraphClient builds a graphClient identical to the one newGraphClient
// produces, except pointed at a local httptest.Server instead of
// graph.microsoft.com, so listUsers/listGroups exercise the real Microsoft
// Graph SDK (request building, JSON parsing, pagination) against canned
// responses instead of a live tenant.
func newTestGraphClient(t *testing.T, baseURL string) *graphClient {
	t.Helper()

	adapter, err := nethttplibrary.NewNetHttpRequestAdapter(noopAuth{})
	require.NoError(t, err)

	adapter.SetBaseUrl(baseURL)

	return &graphClient{client: msgraphsdk.NewGraphServiceClient(adapter)}
}

// jsonHandler serves body as a JSON response on the given mux path.
func jsonHandler(mux *http.ServeMux, path string, body any) {
	mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
}

const (
	fieldValue       = "value"
	fieldDisplayName = "displayName"
	engineeringGroup = "Engineering"
)

// paginationTestServer serves a two-page @odata.nextLink-paginated listing
// under path (page one, linking to path+"2") and path+"2" (page two), each
// containing a single item, so a listUsers/listGroups call against it
// exercises page-iterator following rather than a single response.
func paginationTestServer(t *testing.T, path string, page1Item, page2Item map[string]any) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	var nextLink string

	mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"@odata.nextLink": nextLink,
			fieldValue:        []map[string]any{page1Item},
		})
	})
	jsonHandler(mux, path+"2", map[string]any{
		fieldValue: []map[string]any{page2Item},
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	nextLink = srv.URL + path + "2"

	return srv
}

func TestListUsers(t *testing.T) {
	mux := http.NewServeMux()
	jsonHandler(mux, "/users", map[string]any{
		fieldValue: []map[string]any{
			{
				"id": "u1", fieldDisplayName: "Ada Lovelace", "mail": "ada@example.com", "userPrincipalName": "ada@example.com",
				"memberOf": []map[string]any{
					{"@odata.type": "#microsoft.graph.group", "id": "g1", fieldDisplayName: engineeringGroup},
					{"@odata.type": "#microsoft.graph.directoryRole", "id": "r1", fieldDisplayName: "Admin"},
				},
			},
		},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	users, err := newTestGraphClient(t, srv.URL).listUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 1)

	u := users[0]
	require.Equal(t, "u1", *u.GetId())
	require.Equal(t, "Ada Lovelace", *u.GetDisplayName())
	require.Equal(t, "ada@example.com", *u.GetMail())
	require.Equal(t, "ada@example.com", *u.GetUserPrincipalName())

	t.Run("memberOf is expanded and distinguishes groups from other directory objects", func(t *testing.T) {
		memberOf := u.GetMemberOf()
		require.Len(t, memberOf, 2)

		group, ok := memberOf[0].(models.Groupable)
		require.True(t, ok, "first member should deserialize as a Group")
		require.Equal(t, "g1", *group.GetId())

		_, ok = memberOf[1].(models.Groupable)
		require.False(t, ok, "a directoryRole must not satisfy Groupable")
	})
}

func TestListUsersPagination(t *testing.T) {
	srv := paginationTestServer(t, "/users",
		map[string]any{"id": "u1", fieldDisplayName: "Ada"},
		map[string]any{"id": "u2", fieldDisplayName: "Bob"},
	)

	users, err := newTestGraphClient(t, srv.URL).listUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 2, "results from both pages should be concatenated")
	require.Equal(t, "u1", *users[0].GetId())
	require.Equal(t, "u2", *users[1].GetId())
}

func TestListGroups(t *testing.T) {
	mux := http.NewServeMux()
	jsonHandler(mux, "/groups", map[string]any{
		fieldValue: []map[string]any{
			{"id": "g1", fieldDisplayName: engineeringGroup, "mail": "eng@example.com"},
		},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	groups, err := newTestGraphClient(t, srv.URL).listGroups(t.Context())
	require.NoError(t, err)
	require.Len(t, groups, 1)

	g := groups[0]
	require.Equal(t, "g1", *g.GetId())
	require.Equal(t, engineeringGroup, *g.GetDisplayName())
	require.Equal(t, "eng@example.com", *g.GetMail())
}

func TestListGroupsPagination(t *testing.T) {
	srv := paginationTestServer(t, "/groups",
		map[string]any{"id": "g1", fieldDisplayName: engineeringGroup},
		map[string]any{"id": "g2", fieldDisplayName: "Sales"},
	)

	groups, err := newTestGraphClient(t, srv.URL).listGroups(t.Context())
	require.NoError(t, err)
	require.Len(t, groups, 2, "results from both pages should be concatenated")
	require.Equal(t, "g1", *groups[0].GetId())
	require.Equal(t, "g2", *groups[1].GetId())
}

func TestListUsersAndGroupsHTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/groups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	gc := newTestGraphClient(t, srv.URL)

	_, err := gc.listUsers(t.Context())
	require.Error(t, err)

	_, err = gc.listGroups(t.Context())
	require.Error(t, err)
}
