package entra

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/groups"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/pkg/errors"
)

// graphScopes is the app-only (client credentials) scope for Microsoft Graph.
var graphScopes = []string{"https://graph.microsoft.com/.default"} //nolint:gochecknoglobals

type graphClient struct {
	client *msgraphsdk.GraphServiceClient
}

func newGraphClient(tenantID, clientID, clientSecret string) (graphLister, error) { //nolint:ireturn
	credential, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create azure client secret credential")
	}

	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(credential, graphScopes)
	if err != nil {
		return nil, errors.Wrap(err, "create graph service client")
	}

	return &graphClient{client: client}, nil
}

// listUsers returns all users in the tenant, with each user's group
// memberships expanded via the "memberOf" navigation property so a single
// call covers both users and their group edges.
func (g *graphClient) listUsers(ctx context.Context) ([]*models.User, error) {
	resp, err := g.client.Users().Get(ctx, &users.UsersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UsersRequestBuilderGetQueryParameters{
			Select: []string{"id", "displayName", graphFieldMail, "userPrincipalName"},
			Expand: []string{"memberOf"},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "list users")
	}

	result := make([]*models.User, 0)

	iter, err := graphcore.NewPageIterator[*models.User](resp, g.client.GetAdapter(), models.CreateUserCollectionResponseFromDiscriminatorValue)
	if err != nil {
		return nil, errors.Wrap(err, "create user page iterator")
	}

	if err := iter.Iterate(ctx, func(u *models.User) bool {
		result = append(result, u)
		return true
	}); err != nil {
		return nil, errors.Wrap(err, "iterate users")
	}

	return result, nil
}

// listGroups returns all groups in the tenant.
func (g *graphClient) listGroups(ctx context.Context) ([]*models.Group, error) {
	resp, err := g.client.Groups().Get(ctx, &groups.GroupsRequestBuilderGetRequestConfiguration{
		QueryParameters: &groups.GroupsRequestBuilderGetQueryParameters{
			Select: []string{"id", "displayName", graphFieldMail},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "list groups")
	}

	result := make([]*models.Group, 0)

	iter, err := graphcore.NewPageIterator[*models.Group](resp, g.client.GetAdapter(), models.CreateGroupCollectionResponseFromDiscriminatorValue)
	if err != nil {
		return nil, errors.Wrap(err, "create group page iterator")
	}

	if err := iter.Iterate(ctx, func(gr *models.Group) bool {
		result = append(result, gr)
		return true
	}); err != nil {
		return nil, errors.Wrap(err, "iterate groups")
	}

	return result, nil
}
