package entra

import (
	"context"
	"time"

	common "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsw "github.com/aserto-dev/go-directory/aserto/directory/writer/v3"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	PluginName             string = "entra"
	defaultPollIntervalSec int    = 300
	defaultUserObjectType  string = "user"
	defaultGroupObjectType string = "group"
	defaultMemberRelation  string = "member"

	graphFieldMail string = "mail"
)

type Config struct {
	Enabled bool `json:"enabled"`
	// TenantID, ClientID, ClientSecret identify the Entra ID app registration
	// used for the client-credentials (app-only) Graph API flow.
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`

	PollIntervalSeconds int `json:"poll_interval_seconds"`

	// Directory object/relation type names that synced users, groups, and
	// group memberships are written under.
	UserObjectType  string `json:"user_object_type"`
	GroupObjectType string `json:"group_object_type"`
	MemberRelation  string `json:"member_relation"`
}

// graphLister is the subset of graphClient's behavior sync depends on,
// letting tests substitute a fake in place of a real Microsoft Graph call.
type graphLister interface {
	listUsers(ctx context.Context) ([]*models.User, error)
	listGroups(ctx context.Context) ([]*models.Group, error)
}

type Plugin struct {
	ctx       context.Context
	cancel    context.CancelFunc
	manager   *plugins.Manager
	logger    *zerolog.Logger
	config    *Config
	writer    dsw.WriterClient
	newClient func(tenantID, clientID, clientSecret string) (graphLister, error)
}

func newEntraPlugin(logger *zerolog.Logger, cfg *Config, manager *plugins.Manager, writer dsw.WriterClient) *Plugin {
	newLogger := logger.With().Str("component", "entra.plugin").Logger()

	syncContext, cancel := context.WithCancel(context.Background())

	return &Plugin{
		ctx:       syncContext,
		cancel:    cancel,
		logger:    &newLogger,
		manager:   manager,
		config:    cfg,
		writer:    writer,
		newClient: newGraphClient,
	}
}

func (p *Plugin) Start(ctx context.Context) error {
	p.logger.Info().Str("id", p.manager.ID).Str("tenant_id", p.config.TenantID).Msg("EntraPlugin.Start")

	if err := p.sync(ctx); err != nil {
		p.logger.Error().Err(err).Msg("initial entra sync failed")
		p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateErr, Message: err.Error()})
	} else {
		p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})
	}

	go p.scheduler()

	return nil
}

func (p *Plugin) Stop(ctx context.Context) {
	p.logger.Info().Str("id", p.manager.ID).Msg("EntraPlugin.Stop")

	p.cancel()
	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateNotReady})
}

func (p *Plugin) Reconfigure(ctx context.Context, config any) {
	newConfig, ok := config.(*Config)
	if !ok {
		p.logger.Error().Str("config", "failed type assertion").Msg("EntraPlugin.Reconfigure")
		return
	}

	p.logger.Trace().Str("id", p.manager.ID).Interface("cur", p.config).Interface("new", newConfig).Msg("EntraPlugin.Reconfigure")

	p.config = newConfig
}

func (p *Plugin) scheduler() {
	current := pollInterval(p.config)
	ticker := time.NewTicker(time.Duration(current) * time.Second)

	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if next := pollInterval(p.config); next != current {
				current = next
				ticker.Reset(time.Duration(current) * time.Second)
			}

			if err := p.sync(p.ctx); err != nil {
				p.logger.Error().Err(err).Msg("entra sync failed")
				p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateErr, Message: err.Error()})

				continue
			}

			p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})
		}
	}
}

// sync performs one additive (upsert-only) pass: fetch users and groups from
// Entra ID and upsert them, plus each user's group memberships, into the
// directory. Objects/relations removed on the Entra side are left in place;
// deletion/reconciliation is a follow-up.
func (p *Plugin) sync(ctx context.Context) error {
	graph, err := p.newClient(p.config.TenantID, p.config.ClientID, p.config.ClientSecret)
	if err != nil {
		return errors.Wrap(err, "create graph client")
	}

	groupList, err := graph.listGroups(ctx)
	if err != nil {
		return errors.Wrap(err, "list entra groups")
	}

	for _, g := range groupList {
		if err := p.setGroup(ctx, g); err != nil {
			return errors.Wrapf(err, "sync group '%s'", stringValue(g.GetId()))
		}
	}

	userList, err := graph.listUsers(ctx)
	if err != nil {
		return errors.Wrap(err, "list entra users")
	}

	for _, u := range userList {
		if err := p.setUser(ctx, u); err != nil {
			return errors.Wrapf(err, "sync user '%s'", stringValue(u.GetId()))
		}

		if err := p.syncUserMemberships(ctx, u); err != nil {
			return err
		}
	}

	p.logger.Info().Int("users", len(userList)).Int("groups", len(groupList)).Msg("entra sync complete")

	return nil
}

// syncUserMemberships writes a "member" relation for each of u's group
// memberships. memberOf can include non-group directory objects (e.g.
// directory roles); only group membership is synced.
func (p *Plugin) syncUserMemberships(ctx context.Context, u *models.User) error {
	for _, member := range u.GetMemberOf() {
		group, ok := member.(models.Groupable)
		if !ok {
			continue
		}

		if err := p.setMembership(ctx, group, u); err != nil {
			return errors.Wrapf(err, "sync membership of user '%s' in group '%s'", stringValue(u.GetId()), stringValue(group.GetId()))
		}
	}

	return nil
}

func (p *Plugin) setUser(ctx context.Context, u *models.User) error {
	props, err := structpb.NewStruct(map[string]any{
		"display_name":        stringValue(u.GetDisplayName()),
		graphFieldMail:        stringValue(u.GetMail()),
		"user_principal_name": stringValue(u.GetUserPrincipalName()),
	})
	if err != nil {
		return errors.Wrap(err, "build user properties")
	}

	_, err = p.writer.SetObject(ctx, &dsw.SetObjectRequest{
		Object: &common.Object{
			Type:       userObjectType(p.config),
			Id:         stringValue(u.GetId()),
			Properties: props,
		},
	})

	return errors.Wrap(err, "set object")
}

func (p *Plugin) setGroup(ctx context.Context, g *models.Group) error {
	props, err := structpb.NewStruct(map[string]any{
		"display_name": stringValue(g.GetDisplayName()),
		graphFieldMail: stringValue(g.GetMail()),
	})
	if err != nil {
		return errors.Wrap(err, "build group properties")
	}

	_, err = p.writer.SetObject(ctx, &dsw.SetObjectRequest{
		Object: &common.Object{
			Type:       groupObjectType(p.config),
			Id:         stringValue(g.GetId()),
			Properties: props,
		},
	})

	return errors.Wrap(err, "set object")
}

func (p *Plugin) setMembership(ctx context.Context, group models.Groupable, u *models.User) error {
	_, err := p.writer.SetRelation(ctx, &dsw.SetRelationRequest{
		Relation: &common.Relation{
			ObjectType:  groupObjectType(p.config),
			ObjectId:    stringValue(group.GetId()),
			Relation:    memberRelation(p.config),
			SubjectType: userObjectType(p.config),
			SubjectId:   stringValue(u.GetId()),
		},
	})

	return errors.Wrap(err, "set relation")
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func pollInterval(cfg *Config) int {
	if cfg.PollIntervalSeconds <= 0 {
		return defaultPollIntervalSec
	}

	return cfg.PollIntervalSeconds
}

func userObjectType(cfg *Config) string {
	if cfg.UserObjectType == "" {
		return defaultUserObjectType
	}

	return cfg.UserObjectType
}

func groupObjectType(cfg *Config) string {
	if cfg.GroupObjectType == "" {
		return defaultGroupObjectType
	}

	return cfg.GroupObjectType
}

func memberRelation(cfg *Config) string {
	if cfg.MemberRelation == "" {
		return defaultMemberRelation
	}

	return cfg.MemberRelation
}
