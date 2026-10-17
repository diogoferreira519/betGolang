package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr string
	DatabaseURL string
	MigrationsPath string
	OIDCIssuerURL string
	OIDCAudience string
	AWSRegion string
	AWSEndpoint string
	AWSAccessKey string
	AWSSecretKey string
	SQSWagerQueueURL string
	SQSEventsQueueURL string
	SQSVisibilityTimeout int32
	SQSMaxReceiveCount int
	ShutdownTimeout time.Duration
	PendingRefMaxAttempts int
	PendingRefTTL time.Duration
	OutboxBatchSize int
	OutboxLeaseDuration time.Duration
	InternalClientID string
	MetricsAddr string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		MigrationsPath: getenv("MIGRATIONS_PATH", "file://migrations"),
		OIDCIssuerURL: os.Getenv("OIDC_ISSUER_URL"),
		OIDCAudience: getenv("OIDC_AUDIENCE", "account"),
		AWSRegion: getenv("AWS_REGION", "us-east-1"),
		AWSEndpoint: os.Getenv("AWS_ENDPOINT_URL"),
		AWSAccessKey: getenv("AWS_ACCESS_KEY_ID", "test"),
		AWSSecretKey: getenv("AWS_SECRET_ACCESS_KEY", "test"),
		SQSWagerQueueURL: os.Getenv("SQS_WAGER_QUEUE_URL"),
		SQSEventsQueueURL: os.Getenv("SQS_EVENTS_QUEUE_URL"),
		SQSVisibilityTimeout: int32(getenvInt("SQS_VISIBILITY_TIMEOUT", 30)),
		SQSMaxReceiveCount: getenvInt("SQS_MAX_RECEIVE_COUNT", 5),
		ShutdownTimeout: time.Duration(getenvInt("SHUTDOWN_TIMEOUT_SEC", 20)) * time.Second,
		PendingRefMaxAttempts: getenvInt("PENDING_REF_MAX_ATTEMPTS", 20),
		PendingRefTTL: time.Duration(getenvInt("PENDING_REF_TTL_SEC", 900)) * time.Second,
		OutboxBatchSize: getenvInt("OUTBOX_BATCH_SIZE", 20),
		OutboxLeaseDuration: time.Duration(getenvInt("OUTBOX_LEASE_SEC", 30)) * time.Second,
		InternalClientID: getenv("INTERNAL_CLIENT_ID", "wallet-internal"),
		MetricsAddr: getenv("METRICS_PATH", "/metrics"),
	}
	if cfg.DatabaseURL == "" { return Config{}, fmt.Errorf("DATABASE_URL is required") }
	if cfg.OIDCIssuerURL == "" { return Config{}, fmt.Errorf("OIDC_ISSUER_URL is required") }
	if cfg.SQSWagerQueueURL == "" || cfg.SQSEventsQueueURL == "" { return Config{}, fmt.Errorf("SQS queue URLs are required") }
	return cfg, nil
}

func getenv(k, def string) string { if v := os.Getenv(k); v != "" { return v }; return def }
func getenvInt(k string, def int) int {
	v := os.Getenv(k); if v == "" { return def }
	n, err := strconv.Atoi(v); if err != nil { return def }; return n
}
