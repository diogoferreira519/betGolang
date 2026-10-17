# Architecture

## Overview

Hexagonal layout: pure domain (`internal/domain`), application use cases (`internal/app`), adapters (HTTP, Postgres/pgx, SQS, OIDC), wired by Uber Fx (`internal/fxmodules`).

## Money

- Representation: `int64` minor units (cents), scale 2, ISO 4217 currency on the value object.
- External contract: `{"amount":"25.00","currency":"BRL"}` strings only — no `float32`/`float64` in parse, math, JSON, or SQL.
- Persistence: `BIGINT` minor + `CHAR(3)` currency.
- Overflow checked on parse/add/sub/negate. External inputs reject empty, NaN, Infinity, scientific notation, scale > 2, negatives.
- Currency mismatch fails operations. Scenarios use BRL; incompatibility is tested.

## SQL transaction boundary

A single `pgx.Tx` (unit of work) covers wallet lock/update, wager row, ledger insert, inbox complete, and outbox inserts. Repositories accept `ports.Tx` and never open nested connections on the write path.

## Concurrency

Per-wallet `SELECT … FOR UPDATE` plus conditional `UPDATE … WHERE version = $expected`. No global locks. Distinct wallets proceed in parallel.

## Idempotency

- HTTP: required `Idempotency-Key` (client-supplied; never replaced).
- SQS: `data.idempotencyKey` + inbox `(consumerName, messageId)`.
- Payload hash: SHA-256 of canonical JSON (sorted keys) over business fields: providerId, externalTransactionId, playerId, walletId, roundId, gameId, kind, money amount/currency (normalized to 2 decimals + uppercase currency), optional referenceExternalTransactionId.
- Same key+hash → replay original result (`idempotentReplay: true`) including original observed balance.
- Same key/different hash → 409. Same external id/different key → 409.
- Stored in PostgreSQL; survives full process restart.

## Wager state machine

`PENDING` → `PROCESSED` | `REJECTED` | `FAILED` | `PENDING_REFERENCE`  
`PENDING_REFERENCE` → `PROCESSED` | `REJECTED` | `FAILED`  
Terminals are immutable; replays return persisted results.

Transient infra errors leave SQS visibility for retry. Confirmed business rejections are terminal and allow message delete. Exhausted broker receives → DLQ.

## Pending references

Missing / non-processed reference → `PENDING_REFERENCE` + outbox event. Worker uses exponential backoff (1s…60s), max 20 attempts / 15m TTL → `REJECTED` (`REFERENCE_NOT_FOUND`). Reference terminal without success → `REFERENCE_NOT_PROCESSED`.

## REFUND × ROLLBACK

A BET debit may be restored at most once (REFUND or ROLLBACK of that BET). Unique index on `(reference_internal_id, kind)` for successful reversals; application also counts successful restorations. ROLLBACK of WIN/REFUND debits; insufficient balance uses `INSUFFICIENT_FUNDS_REVERSAL` (distinct from BET `INSUFFICIENT_FUNDS`).

## Inbox / outbox

- Inbox uniqueness `(consumer_name, message_id)`; hash checked on redelivery; complete in same commit as domain effects (including PENDING_REFERENCE).
- Outbox rows written in the same commit; publisher claims with `FOR UPDATE SKIP LOCKED` + lease; republish preserves `eventId`. Events: Processed, Rejected, BalanceChanged, PendingReference.

## AuthN / AuthZ

- Keycloak OIDC; JWT validated via JWKS (`coreos/go-oidc`).
- Clients: `provider-a`, `provider-b`, `wallet-internal` (client_credentials).
- `azp` / client id = `providerId`. Providers may only access their own transactions. Wallet open/get/ledger/reconcile require internal client (`wallet:admin` or `wallet-internal`).
- No local password store or token minting.

## Fx & shutdown

Modules provide config, pool, repos, use cases, HTTP, workers. Lifecycle: validate/migrate → start HTTP + workers → on stop: stop accept/poll, drain with timeout, close pool.

## Ledger

Append-only table; trigger forbids UPDATE/DELETE; uniqueness `(wallet_id, transaction_id)`; balance continuity checks in constraints.

## Limitations

- Keycloak service-account realm roles are not auto-assigned in the import JSON beyond client definitions; isolation relies on client id as providerId.
- Integration suite expects Compose dependencies; unit tests do not.
- Optional doubles / OpenTelemetry tracing not implemented.
