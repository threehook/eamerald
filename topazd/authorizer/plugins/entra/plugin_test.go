//nolint:testpackage
package entra

import (
	"context"
	"testing"
	"time"

	common "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	testEmployeeObjectType = "employee"
	testTeamObjectType     = "team"
)

// fakeWriter is an in-memory dsw.WriterClient that records every call, so
// tests can assert on what the plugin would have written to the directory
// without a real directory or network access.
type fakeWriter struct {
	objects   []*common.Object
	relations []*common.Relation
	err       error
}

var _ dsw.WriterClient = (*fakeWriter)(nil)

func (f *fakeWriter) SetObject(_ context.Context, in *dsw.SetObjectRequest, _ ...grpc.CallOption) (*dsw.SetObjectResponse, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.objects = append(f.objects, in.GetObject())

	return &dsw.SetObjectResponse{Result: in.GetObject()}, nil
}

func (f *fakeWriter) DeleteObject(_ context.Context, _ *dsw.DeleteObjectRequest, _ ...grpc.CallOption) (*dsw.DeleteObjectResponse, error) {
	return &dsw.DeleteObjectResponse{}, f.err
}

func (f *fakeWriter) SetRelation(_ context.Context, in *dsw.SetRelationRequest, _ ...grpc.CallOption) (*dsw.SetRelationResponse, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.relations = append(f.relations, in.GetRelation())

	return &dsw.SetRelationResponse{Result: in.GetRelation()}, nil
}

func (f *fakeWriter) DeleteRelation(_ context.Context, _ *dsw.DeleteRelationRequest, _ ...grpc.CallOption) (*dsw.DeleteRelationResponse, error) {
	return &dsw.DeleteRelationResponse{}, f.err
}

// fakeGraphLister is a canned graphLister, so sync can be tested without a
// real Microsoft Graph client or network access.
type fakeGraphLister struct {
	users     []*models.User
	groups    []*models.Group
	usersErr  error
	groupsErr error
}

var _ graphLister = (*fakeGraphLister)(nil)

func (f *fakeGraphLister) listUsers(_ context.Context) ([]*models.User, error) {
	return f.users, f.usersErr
}

func (f *fakeGraphLister) listGroups(_ context.Context) ([]*models.Group, error) {
	return f.groups, f.groupsErr
}

func testUser(id, displayName, mail, upn string) *models.User {
	u := models.NewUser()
	u.SetId(&id)
	u.SetDisplayName(&displayName)
	u.SetMail(&mail)
	u.SetUserPrincipalName(&upn)

	return u
}

func testGroup(id, displayName, mail string) *models.Group {
	g := models.NewGroup()
	g.SetId(&id)
	g.SetDisplayName(&displayName)
	g.SetMail(&mail)

	return g
}

func TestSetUser(t *testing.T) {
	writer := &fakeWriter{}
	p := &Plugin{config: &Config{}, writer: writer}

	user := testUser("u1", "Ada Lovelace", "ada@example.com", "ada@example.com")

	require.NoError(t, p.setUser(t.Context(), user))
	require.Len(t, writer.objects, 1)

	obj := writer.objects[0]
	require.Equal(t, "user", obj.GetType(), "defaults to the default user object type")
	require.Equal(t, "u1", obj.GetId())

	props := obj.GetProperties().AsMap()
	require.Equal(t, "Ada Lovelace", props["display_name"])
	require.Equal(t, "ada@example.com", props["mail"])
	require.Equal(t, "ada@example.com", props["user_principal_name"])

	require.Len(t, writer.relations, 1)

	rel := writer.relations[0]
	require.Equal(t, "user", rel.GetObjectType())
	require.Equal(t, "u1", rel.GetObjectId())
	require.Equal(t, "identifier", rel.GetRelation())
	require.Equal(t, "identity", rel.GetSubjectType())
	require.Equal(t, "ada@example.com", rel.GetSubjectId(), "keyed on the UPN")

	t.Run("respects a configured user object type", func(t *testing.T) {
		writer := &fakeWriter{}
		p := &Plugin{config: &Config{UserObjectType: testEmployeeObjectType}, writer: writer}

		require.NoError(t, p.setUser(t.Context(), user))
		require.Equal(t, testEmployeeObjectType, writer.objects[0].GetType())
	})

	t.Run("tolerates a user with unset optional fields", func(t *testing.T) {
		writer := &fakeWriter{}
		p := &Plugin{config: &Config{}, writer: writer}

		bare := models.NewUser()
		bare.SetId(new("u2"))

		require.NoError(t, p.setUser(t.Context(), bare))
		require.Empty(t, writer.objects[0].GetProperties().AsMap()["mail"])
		require.Empty(t, writer.relations, "no identifier relation without a UPN")
	})

	t.Run("propagates writer errors", func(t *testing.T) {
		writer := &fakeWriter{err: errors.New("boom")}
		p := &Plugin{config: &Config{}, writer: writer}

		require.Error(t, p.setUser(t.Context(), user))
	})
}

func TestSetGroup(t *testing.T) {
	writer := &fakeWriter{}
	p := &Plugin{config: &Config{}, writer: writer}

	group := testGroup("g1", "Engineering", "eng@example.com")

	require.NoError(t, p.setGroup(t.Context(), group))
	require.Len(t, writer.objects, 1)

	obj := writer.objects[0]
	require.Equal(t, "group", obj.GetType(), "defaults to the default group object type")
	require.Equal(t, "g1", obj.GetId())

	props := obj.GetProperties().AsMap()
	require.Equal(t, "Engineering", props["display_name"])
	require.Equal(t, "eng@example.com", props["mail"])

	t.Run("respects a configured group object type", func(t *testing.T) {
		writer := &fakeWriter{}
		p := &Plugin{config: &Config{GroupObjectType: testTeamObjectType}, writer: writer}

		require.NoError(t, p.setGroup(t.Context(), group))
		require.Equal(t, testTeamObjectType, writer.objects[0].GetType())
	})

	t.Run("propagates writer errors", func(t *testing.T) {
		writer := &fakeWriter{err: errors.New("boom")}
		p := &Plugin{config: &Config{}, writer: writer}

		require.Error(t, p.setGroup(t.Context(), group))
	})
}

func TestSetMembership(t *testing.T) {
	writer := &fakeWriter{}
	p := &Plugin{config: &Config{}, writer: writer}

	group := testGroup("g1", "Engineering", "eng@example.com")
	user := testUser("u1", "Ada Lovelace", "ada@example.com", "ada@example.com")

	require.NoError(t, p.setMembership(t.Context(), group, user))
	require.Len(t, writer.relations, 1)

	rel := writer.relations[0]
	require.Equal(t, "group", rel.GetObjectType())
	require.Equal(t, "g1", rel.GetObjectId())
	require.Equal(t, "member", rel.GetRelation(), "defaults to the default member relation name")
	require.Equal(t, "user", rel.GetSubjectType())
	require.Equal(t, "u1", rel.GetSubjectId())

	t.Run("respects configured type and relation names", func(t *testing.T) {
		writer := &fakeWriter{}
		p := &Plugin{config: &Config{
			GroupObjectType: testTeamObjectType,
			UserObjectType:  testEmployeeObjectType,
			MemberRelation:  "belongs_to",
		}, writer: writer}

		require.NoError(t, p.setMembership(t.Context(), group, user))

		rel := writer.relations[0]
		require.Equal(t, testTeamObjectType, rel.GetObjectType())
		require.Equal(t, "belongs_to", rel.GetRelation())
		require.Equal(t, testEmployeeObjectType, rel.GetSubjectType())
	})

	t.Run("propagates writer errors", func(t *testing.T) {
		writer := &fakeWriter{err: errors.New("boom")}
		p := &Plugin{config: &Config{}, writer: writer}

		require.Error(t, p.setMembership(t.Context(), group, user))
	})
}

func TestSyncUserMemberships(t *testing.T) {
	writer := &fakeWriter{}
	p := &Plugin{config: &Config{}, writer: writer}

	group := testGroup("g1", "Engineering", "eng@example.com")
	role := models.NewDirectoryRole()
	role.SetId(new("r1"))

	user := testUser("u1", "Ada Lovelace", "ada@example.com", "ada@example.com")
	user.SetMemberOf([]models.DirectoryObjectable{group, role})

	require.NoError(t, p.syncUserMemberships(t.Context(), user))

	require.Len(t, writer.relations, 1, "only the group membership should be synced, not the directory role")
	require.Equal(t, "g1", writer.relations[0].GetObjectId())

	t.Run("propagates writer errors", func(t *testing.T) {
		writer := &fakeWriter{err: errors.New("boom")}
		p := &Plugin{config: &Config{}, writer: writer}

		require.Error(t, p.syncUserMemberships(t.Context(), user))
	})
}

func newClientFunc(lister graphLister, err error) func(string, string, string) (graphLister, error) {
	return func(string, string, string) (graphLister, error) { return lister, err }
}

func TestSync(t *testing.T) {
	logger := zerolog.Nop()

	group1 := testGroup("g1", "Engineering", "eng@example.com")
	group2 := testGroup("g2", "Sales", "sales@example.com")

	user1 := testUser("u2", "Bob Smith", "bob@example.com", "bob@example.com")
	user1.SetMemberOf([]models.DirectoryObjectable{group1})

	t.Run("happy path syncs groups before users, including memberships", func(t *testing.T) {
		writer := &fakeWriter{}
		lister := &fakeGraphLister{groups: []*models.Group{group1, group2}, users: []*models.User{user1}}
		p := &Plugin{config: &Config{}, writer: writer, newClient: newClientFunc(lister, nil), logger: &logger}

		require.NoError(t, p.sync(t.Context()))

		require.Len(t, writer.objects, 3, "2 groups + 1 user")
		require.Equal(t, "g1", writer.objects[0].GetId(), "groups are synced before users")
		require.Equal(t, "g2", writer.objects[1].GetId())
		require.Equal(t, "u2", writer.objects[2].GetId())

		require.Len(t, writer.relations, 2, "1 identifier relation for the user + 1 group membership")
		require.Equal(t, "u2", writer.relations[0].GetObjectId(), "identifier relation is written by setUser")
		require.Equal(t, "identifier", writer.relations[0].GetRelation())
		require.Equal(t, "g1", writer.relations[1].GetObjectId(), "membership relation is written by syncUserMemberships")
		require.Equal(t, "member", writer.relations[1].GetRelation())
	})

	t.Run("propagates a graph client creation error", func(t *testing.T) {
		p := &Plugin{config: &Config{}, writer: &fakeWriter{}, newClient: newClientFunc(nil, errors.New("boom"))}
		require.Error(t, p.sync(t.Context()))
	})

	t.Run("a listGroups error short-circuits before any user work", func(t *testing.T) {
		writer := &fakeWriter{}
		lister := &fakeGraphLister{groupsErr: errors.New("boom"), users: []*models.User{user1}}
		p := &Plugin{config: &Config{}, writer: writer, newClient: newClientFunc(lister, nil)}

		require.Error(t, p.sync(t.Context()))
		require.Empty(t, writer.objects, "no groups or users should be written")
	})

	t.Run("a listUsers error is returned after groups have already been synced", func(t *testing.T) {
		writer := &fakeWriter{}
		lister := &fakeGraphLister{groups: []*models.Group{group1}, usersErr: errors.New("boom")}
		p := &Plugin{config: &Config{}, writer: writer, newClient: newClientFunc(lister, nil)}

		require.Error(t, p.sync(t.Context()))
		require.Len(t, writer.objects, 1, "the group should already have been written")
	})

	t.Run("a setGroup failure stops the sync and reports the failing group id", func(t *testing.T) {
		writer := &fakeWriter{err: errors.New("boom")}
		lister := &fakeGraphLister{groups: []*models.Group{group1, group2}}
		p := &Plugin{config: &Config{}, writer: writer, newClient: newClientFunc(lister, nil)}

		err := p.sync(t.Context())
		require.ErrorContains(t, err, "g1")
	})

	t.Run("a setUser failure stops the sync and reports the failing user id", func(t *testing.T) {
		writer := &fakeWriter{err: errors.New("boom")}
		lister := &fakeGraphLister{users: []*models.User{user1}}
		p := &Plugin{config: &Config{}, writer: writer, newClient: newClientFunc(lister, nil)}

		err := p.sync(t.Context())
		require.ErrorContains(t, err, "u2")
	})
}

// blockingGraphLister.listGroups blocks until unblock is closed, so tests can
// prove Start returns before a slow/stuck sync completes.
type blockingGraphLister struct {
	unblock chan struct{}
}

var _ graphLister = (*blockingGraphLister)(nil)

func (b *blockingGraphLister) listGroups(ctx context.Context) ([]*models.Group, error) {
	select {
	case <-b.unblock:
	case <-ctx.Done():
	}

	return nil, ctx.Err()
}

func (b *blockingGraphLister) listUsers(_ context.Context) ([]*models.User, error) {
	return nil, nil
}

// TestStartReportsOKWithoutSyncing covers the startup deadlock: the OPA
// runtime only becomes ready once every plugin reports StateOK, and the
// directory services the sync writes to only start serving after that. Start
// must therefore both return and report StateOK without waiting on a sync.
func TestStartReportsOKWithoutSyncing(t *testing.T) {
	logger := zerolog.Nop()
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	unblock := make(chan struct{})

	t.Cleanup(func() { close(unblock) })

	p := newEntraPlugin(&logger, &Config{PollIntervalSeconds: 3600}, mgr, &fakeWriter{})
	p.newClient = newClientFunc(&blockingGraphLister{unblock: unblock}, nil)

	done := make(chan error, 1)
	go func() { done <- p.Start(t.Context()) }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Start blocked on sync instead of returning immediately")
	}

	require.Equal(t, plugins.StateOK, mgr.PluginStatus()[PluginName].State,
		"Start must report StateOK without waiting for a sync, otherwise the runtime never becomes ready")

	p.Stop(t.Context())

	require.Equal(t, plugins.StateNotReady, mgr.PluginStatus()[PluginName].State)
}

// TestRunSyncDoesNotReportStateErr guards the runtime's post-start plugin
// status check: a failing sync must not leave the plugin in StateErr, which
// would fail the boot over a transient Graph or directory error.
func TestRunSyncDoesNotReportStateErr(t *testing.T) {
	logger := zerolog.Nop()
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	p := newEntraPlugin(&logger, &Config{}, mgr, &fakeWriter{err: errors.New("boom")})
	p.newClient = newClientFunc(&fakeGraphLister{users: []*models.User{testUser("u1", "", "", "")}}, nil)

	require.NoError(t, p.Start(t.Context()))

	// stop the scheduler, so its initial sync cannot race the one below.
	p.cancel()

	p.runSync()

	require.Equal(t, plugins.StateOK, mgr.PluginStatus()[PluginName].State)
}

func TestConfigDefaults(t *testing.T) {
	require.Equal(t, defaultPollIntervalSec, pollInterval(&Config{}))
	require.Equal(t, 60, pollInterval(&Config{PollIntervalSeconds: 60}))
	require.Equal(t, defaultPollIntervalSec, pollInterval(&Config{PollIntervalSeconds: -1}))

	require.Equal(t, defaultUserObjectType, userObjectType(&Config{}))
	require.Equal(t, testEmployeeObjectType, userObjectType(&Config{UserObjectType: testEmployeeObjectType}))

	require.Equal(t, defaultGroupObjectType, groupObjectType(&Config{}))
	require.Equal(t, testTeamObjectType, groupObjectType(&Config{GroupObjectType: testTeamObjectType}))

	require.Equal(t, defaultMemberRelation, memberRelation(&Config{}))
	require.Equal(t, "belongs_to", memberRelation(&Config{MemberRelation: "belongs_to"}))
}

func TestStringValue(t *testing.T) {
	require.Empty(t, stringValue(nil))
	require.Equal(t, "x", stringValue(new("x")))
}

func TestReconfigure(t *testing.T) {
	logger := zerolog.Nop()
	mgr, err := plugins.New([]byte("{}"), "test", inmem.New())
	require.NoError(t, err)

	p := newEntraPlugin(&logger, &Config{TenantID: "old"}, mgr, &fakeWriter{})

	newCfg := &Config{TenantID: "new"}
	p.Reconfigure(context.Background(), newCfg)
	require.Same(t, newCfg, p.config)

	t.Run("ignores a malformed config", func(t *testing.T) {
		p.Reconfigure(context.Background(), "not a *Config")
		require.Same(t, newCfg, p.config, "config should be left untouched")
	})
}
