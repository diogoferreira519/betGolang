# golangbet — Distributed Wager Processing

Go service with Uber Fx that processes provider wagering operations over HTTP and SQS with PostgreSQL ledger guarantees, Keycloak OIDC, and LocalStack FIFO queues.

## Prerequisites

- Go 1.22+
- Docker / Docker Compose
- `curl` (for examples)

## Quick start

```bash
cp .env.example .env
chmod +x deploy/localstack/init-queues.sh
docker compose up --build
```

API: `http://localhost:8080`  
Keycloak: `http://localhost:8081` (admin/admin)  
LocalStack: `http://localhost:4566`

### Migrations

Applied automatically on API startup (`MIGRATIONS_PATH`).

Manual (host tooling):

```bash
migrate -path migrations -database "$DATABASE_URL" up
migrate -path migrations -database "$DATABASE_URL" down 1
```

### Obtain tokens (client_credentials)

```bash
TOKEN=$(curl -s -X POST "http://localhost:8081/realms/golangbet/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=wallet-internal" \
  -d "client_secret=wallet-internal-secret" | jq -r .access_token)

PROVIDER_TOKEN=$(curl -s -X POST "http://localhost:8081/realms/golangbet/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=provider-a" \
  -d "client_secret=provider-a-secret" | jq -r .access_token)
```

### Example calls

```bash
# Open wallet (internal)
curl -s -X POST http://localhost:8080/wallets \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","initialBalance":{"amount":"1000.00","currency":"BRL"}}'

# BET (provider)
curl -s -X POST http://localhost:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H "Idempotency-Key: provider-a:transaction-123" \
  -H "Content-Type: application/json" \
  -d '{"providerId":"provider-a","externalTransactionID":"transaction-123","playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","walletId":"<WALLET_ID>","roundId":"round-987","gameId":"fortune-chimp","kind":"BET","money":{"amount":"25.00","currency":"BRL"}}'
```

Note: JSON field is `externalTransactionId` (camelCase) as in the challenge contract.

### Queues

Created by LocalStack init script:

- `wager-transactions.fifo` (+ DLQ redrive, maxReceiveCount=5, visibility 30s)
- `domain-events.fifo` (outbox destination)

`MessageGroupId` = `walletId`; `MessageDeduplicationId` = business hash / `eventId`.

## Tests

```bash
go test ./...
go test -race ./...
go vet ./...

# Integration (requires docker compose deps)
go test -tags=integration ./test/integration/...
```

## Multi-instance / failure drills

```bash
docker compose up --build --scale api=3
# kill one api container mid-processing; redeliveries and outbox lease recovery should preserve balances
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for money, locks, idempotency, inbox/outbox, authz, and shutdown.
