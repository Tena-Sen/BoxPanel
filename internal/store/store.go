// Package store defines the persistence interface for all domain objects.
//
// 仓储层抽象，便于后续替换实现（如换 Postgres/远程存储）。
// 当前实现见 internal/store/sqlite。
package store

import (
	"context"

	"boxpanel/internal/models"
)

// Store is the aggregate persistence interface.
type Store interface {
	// Servers
	ListServers(ctx context.Context) ([]models.Server, error)
	GetServer(ctx context.Context, id string) (*models.Server, error)
	SaveServer(ctx context.Context, s models.Server) error
	DeleteServer(ctx context.Context, id string) error
	BatchSaveServers(ctx context.Context, servers []models.Server) error

	// Groups
	ListGroups(ctx context.Context) ([]models.Group, error)
	GetGroup(ctx context.Context, id string) (*models.Group, error)
	SaveGroup(ctx context.Context, g models.Group) error
	DeleteGroup(ctx context.Context, id string) error

	// Subscriptions
	ListSubscriptions(ctx context.Context) ([]models.Subscription, error)
	GetSubscription(ctx context.Context, id string) (*models.Subscription, error)
	SaveSubscription(ctx context.Context, s models.Subscription) error
	DeleteSubscription(ctx context.Context, id string) error

	// Routing rules
	ListRoutingRules(ctx context.Context, profileID string) ([]models.RoutingRule, error)
	SaveRoutingRule(ctx context.Context, r models.RoutingRule) error
	DeleteRoutingRule(ctx context.Context, id string) error
	ReorderRoutingRules(ctx context.Context, profileID string, ids []string) error

	// Rule sets
	ListRuleSets(ctx context.Context) ([]models.RuleSet, error)
	GetRuleSet(ctx context.Context, id string) (*models.RuleSet, error)
	SaveRuleSet(ctx context.Context, r models.RuleSet) error
	DeleteRuleSet(ctx context.Context, id string) error

	// Profiles
	ListProfiles(ctx context.Context) ([]models.Profile, error)
	GetProfile(ctx context.Context, id string) (*models.Profile, error)
	SaveProfile(ctx context.Context, p models.Profile) error
	DeleteProfile(ctx context.Context, id string) error

	// Settings
	GetSettings(ctx context.Context) (models.Settings, error)
	SaveSettings(ctx context.Context, s models.Settings) error

	// Lifecycle
	Close() error
}
