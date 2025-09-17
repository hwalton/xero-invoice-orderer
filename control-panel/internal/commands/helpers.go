package commands

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func connectDB(ctx context.Context, dbURL string) (*pgx.Conn, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	return pgx.Connect(ctx, dbURL)
}
