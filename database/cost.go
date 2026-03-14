package database

import (
	"context"
	"fmt"
)

type CostEstimate struct {
	EstimatedRows int
	EstimatedCost int
}

func EstimateQueryCost(ctx context.Context, client Client, query string) (*CostEstimate, error) {
	// TODO: implement
	fmt.Println("TODO: estimate query cost")
	return &CostEstimate{}, nil
}
