# ADR 0001: The deliveries table is the transactional outbox

- Status: Accepted (2026-10-17)
- Date: 2026-10-17

## Context

Event ingestion writes three things in one transaction: the event, one
`deliveries` row per enabled endpoint, and one `outbox_events` row
(`event.accepted`).

The worker (Phase 4) claims work directly from `deliveries` with
`FOR UPDATE SKIP LOCKED`, leases and lease tokens. Nothing reads
`outbox_events`. In the development database every outbox row stays
unprocessed, and because `outbox_events.aggregate_id` has no foreign key, the
rows survive event and tenant deletion.

The transactional outbox pattern exists to avoid a dual write: committing
state to the database and then publishing to a separate broker. Hooklane has no
broker (Redis and Kafka are explicit non-goals). The work items are written in
the same transaction as the event and are consumed from the same database.

## Decision

Treat `deliveries` as the outbox. A `pending` delivery with a due
`next_attempt_at` is an unpublished outbox entry; claiming it is the relay step;
`delivery_attempts` is the publication log.

Stop writing `outbox_events` and drop the table in a new migration. Ingestion
atomicity is still guaranteed by the event and delivery inserts sharing one
transaction, which the existing integration tests cover.

## Alternatives considered

1. **Keep `outbox_events` and add a relay that marks rows processed.** This adds
   a polling loop whose only effect is updating `processed_at`. It costs a
   write per event and gives no delivery guarantee the deliveries table lacks.
2. **Make the outbox the only queue and create deliveries from it.** This adds a
   second asynchronous hop and a window where an accepted event has no visible
   deliveries, which makes `delivery_count` in the ingest response unknowable.
3. **Keep `outbox_events` for future external integrations**, such as streaming
   events to a customer's broker. This is speculative infrastructure; if it
   becomes a requirement, a new migration can reintroduce a purpose-built table.

## Consequences

- One less write per accepted event and no unbounded table without a consumer.
- Phase 3's scope line "persist ... outbox records transactionally" is met by
  deliveries; the phase plan and architecture plan wording should be updated.
- Workers wake up by polling (`WORKER_POLL_INTERVAL`). If latency ever matters,
  `pg_notify` from the ingest transaction can wake idle workers without a
  separate table.
- Dropping the table deletes existing outbox rows. They have never been read,
  so no behaviour depends on them.
