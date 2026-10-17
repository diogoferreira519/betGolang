//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/diogo/golangbet/internal/adapters/postgres"
	"github.com/diogo/golangbet/internal/fxmodules"
	"go.uber.org/fx"
)

func TestMigrationsAndFxLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := postgres.RunMigrations(dsn, "file://../../migrations"); err != nil {
		t.Fatal(err)
	}
	app := fx.New(
		fxmodules.Module,
		fx.NopLogger,
	)
	if err := app.Start(ctx); err != nil {
		t.Fatalf("fx start: %v", err)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("fx stop: %v", err)
	}
}
