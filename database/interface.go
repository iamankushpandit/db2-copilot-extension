package database

import (
	"context"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

// Client is the generic interface for a database client.
type Client interface {
	GetTier1Schema(ctx context.Context) (*Schema, error)
	GetTier2Schema(ctx context.Context, accessConfig *config.AccessConfig) (*Schema, error)
	ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error)
	EstimateQueryCost(ctx context.Context, query string) (*CostEstimate, error)
	VerifyReadOnly() error
	Ping(ctx context.Context) error
	Close() error
}
