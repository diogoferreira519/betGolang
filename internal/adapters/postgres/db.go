package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct{ *pgxpool.Pool }

func NewPool(ctx context.Context, databaseURL string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Pool{Pool: pool}, nil
}

func (p *Pool) Close() { p.Pool.Close() }

type unitOfWork struct{ pool *pgxpool.Pool }

func NewUnitOfWork(pool *Pool) ports.UnitOfWork { return &unitOfWork{pool: pool.Pool} }

type Tx struct{ pgx.Tx }

func (u *unitOfWork) Begin(ctx context.Context) (ports.Tx, error) {
	t, err := u.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: t}, nil
}

func AsTx(t ports.Tx) (pgx.Tx, error) {
	x, ok := t.(*Tx)
	if !ok {
		return nil, fmt.Errorf("invalid tx type")
	}
	return x.Tx, nil
}
