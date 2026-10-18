# Deploying Hooklane

Hooklane runs as two long-lived processes that share one PostgreSQL database:

- **API** (`hooklane-api`): the management API, event ingestion and, when
  `WEB_DIR` is set, the dashboard. Stateless; run as many as you need behind
  a load balancer.
- **Worker** (`hooklane-worker`): claims due deliveries, sends them, records
  attempts and prunes old events. Run one or more; workers coordinate through
  row locks (`FOR UPDATE SKIP LOCKED`) and leases, so adding workers adds
  throughput without double delivery.

`hooklane-admin` applies migrations, creates the first organization and
prunes on demand. All three ship in one image,
`ghcr.io/0furkancolak/hooklane:<version>`, and as release binaries.

## Install in one command

On a host with Docker and Docker Compose v2:

```sh
curl -fsSL https://raw.githubusercontent.com/0furkancolak/hooklane/vX.Y.Z/scripts/install.sh \
  | sh -s -- --version vX.Y.Z
```

Use the release tag you want in both places; the script downloads the deploy
files that belong to that tag. From a checkout, run `./scripts/install.sh`
instead (it uses the files in `deploy/`).

The script checks Docker, creates `./hooklane` (change with `--dir`), writes
`compose.yaml` and the proxy configuration there, generates `.env` with a
random `POSTGRES_PASSWORD` and `ENDPOINT_SECRET_KEY`, starts the stack, waits
for `/readyz`, creates the first organization and prints its API key once at
the end. The key is read from the bootstrap command's output, never passed on a
command line.

| Option | Meaning |
| --- | --- |
| `--version TAG` | Image tag to run (default: the latest release; `latest` from a checkout). |
| `--dir PATH` | Deploy directory (default `./hooklane`). |
| `--port PORT` | Host port of the proxy, bound to `127.0.0.1` (default `8090`). |
| `--no-start` | Write the files and secrets, start nothing. |
| `--project NAME` | Docker Compose project name (default: the directory name). |
| `--api-replicas N`, `--worker-replicas N` | Initial replica counts. |
| `--org-name`, `--org-slug` | The first organization. |

Re-running is safe: an existing `.env` is never regenerated or overwritten
(only options you pass again, such as `--version`, update it), an existing
database volume is reused, and no second organization is created. If
`compose.yaml` changed, the previous copy is saved next to it with a `.bak`
suffix.

Back up `ENDPOINT_SECRET_KEY` from `<dir>/.env` as soon as the script has run
(see [Backups](#backups)). The script tells you to, and never prints it.

The proxy listens on `127.0.0.1`. Put a TLS terminator (Caddy, a cloud load
balancer, another nginx) in front of it, or set `HOOKLANE_BIND=0.0.0.0` in
`.env` only on a network you trust.

To stop the stack, run `./scripts/uninstall.sh --dir <dir>`. It removes the
containers and keeps the data volume. `--purge-data` also deletes the volume,
after you type the directory's name; there is no way to skip that prompt.

## Requirements

- PostgreSQL. Version 17 is the one CI and development test against; the
  schema uses only features available since PostgreSQL 13
  (`gen_random_uuid()`, partial indexes, `FOR UPDATE SKIP LOCKED`).
- A 32-byte key for encrypting endpoint secrets:
  `openssl rand -base64 32`. Keep it with your other secrets and back it up:
  without it, stored endpoint secrets cannot be decrypted and deliveries to
  those endpoints dead-letter.
- TLS termination in front of the API (a reverse proxy or load balancer).
  The API speaks plain HTTP.

## First deployment

1. Create a database and a role that owns it.
2. Apply migrations:
   `DATABASE_URL=... hooklane-admin migrate up`
3. Create the first organization, project and API key:
   `DATABASE_URL=... hooklane-admin bootstrap -org-name Acme -org-slug acme`
   The API key is printed once.
4. Start the API and at least one worker with the variables below.
5. Point your probes at `/healthz` (liveness) and `/readyz` (readiness).

The Compose file's `app` profile does exactly this for a single host:
`docker compose --profile app up -d --build`.

## Running distributed

The API and the worker are separate processes with no local state, so you can
run several of each, on one host or many, against one PostgreSQL. The
production Compose file (`deploy/compose.production.yaml`, installed as
`compose.yaml`) is built for that:

```sh
docker compose up -d --scale api=3 --scale worker=3
```

Only the nginx `proxy` service publishes a port. It resolves `api` through
Docker's DNS again every few seconds, so replicas added or removed with
`--scale` are picked up without restarting it. `api` and `worker` wait for the
one-shot `migrate` service, which runs once per `docker compose up`.
Kubernetes manifests are in `deploy/kubernetes/` (see below).

What is safe to scale, and how it coordinates:

- **API replicas** hold no state. Authentication, ingestion and reads go
  straight to PostgreSQL, so any replica can serve any request.
- **Worker replicas** claim deliveries with a single statement that selects
  due rows `FOR UPDATE SKIP LOCKED` and marks them `processing` with a lease
  (`WORKER_REQUEST_TIMEOUT` + 30s). Concurrent workers get disjoint rows. If a
  worker dies, its rows become claimable again when the lease expires, so a
  crash costs a retry, never a lost delivery. Delivery is at-least-once: a
  worker that loses its lease discards its result and the delivery may be
  sent twice. Receivers should deduplicate on the `webhook-id` header.
- **Retention** runs inside every worker (first after one minute, then
  hourly). Deleting expired events uses the same `FOR UPDATE SKIP LOCKED`
  batching, so workers pruning at once skip each other's rows and never
  block or double-delete. N workers simply prune N times an hour, which is
  redundant but harmless. Events that still have pending or processing
  deliveries are kept.
- **Migrations** are not guarded by a lock. Run `migrate up` from exactly one
  place at a time (the `migrate` service or Job); do not start two at once.
- **Rate limiting** is per API replica and keyed by the connecting address,
  and the API does not trust `X-Forwarded-For`. Behind a proxy that address
  is the proxy, so every client would share one bucket per replica. The
  production Compose file therefore sets the API's limit to 0 and enforces
  `HOOKLANE_RATE_LIMIT_RPS` (default 50 per client address, burst 200) in
  nginx, across all replicas. If you put another balancer in front of that
  proxy, configure `set_real_ip_from` there or all clients look alike.
- **Metrics** are per process. Set `METRICS_ADDR` (the Compose file does) and
  scrape every replica; sum counters across them. Do not publish the metrics
  port.
- **Graceful shutdown**: on `SIGTERM` the API stops accepting connections and
  finishes requests for up to 10 seconds; a worker stops claiming and drains
  in-flight deliveries for `WORKER_SHUTDOWN_TIMEOUT`. Set the orchestrator's
  grace period at least 10 seconds higher (`stop_grace_period` in Compose,
  `terminationGracePeriodSeconds` in Kubernetes), otherwise a worker is killed
  mid-delivery and its deliveries wait for the lease to expire.
- **Database connections**: every process opens its own pool (default
  max(4, CPUs)). With many replicas, size PostgreSQL's `max_connections` for
  the total or put PgBouncer in front.

### Kubernetes

`deploy/kubernetes/` holds plain manifests and a `kustomization.yaml`: a
namespace, ConfigMap, migration Job, API Deployment (2 replicas, `/readyz`
readiness, `/healthz` liveness, non-root, read-only root filesystem), Service,
worker Deployment (2 replicas, 40s termination grace for the default 20s
drain), PodDisruptionBudgets, an API HorizontalPodAutoscaler and an optional
Ingress. PostgreSQL is not included; use a managed instance.

```sh
kubectl apply -f deploy/kubernetes/namespace.yaml
kubectl -n hooklane create secret generic hooklane-secrets \
  --from-literal=DATABASE_URL='postgres://USER:PASSWORD@HOST:5432/hooklane?sslmode=require' \
  --from-literal=ENDPOINT_SECRET_KEY="$(openssl rand -base64 32)"
cd deploy/kubernetes && kustomize edit set image ghcr.io/0furkancolak/hooklane=ghcr.io/0furkancolak/hooklane:vX.Y.Z
kubectl apply -f configmap.yaml -f migrate-job.yaml   # image tag: edit it, or apply -k once
kubectl -n hooklane wait --for=condition=complete job/hooklane-migrate --timeout=5m
kubectl apply -k .
```

Because a Job's template is immutable, `kubectl -n hooklane delete job
hooklane-migrate --ignore-not-found` before applying a new release. The first
organization and API key come from
`kubectl -n hooklane run bootstrap --rm -it --restart=Never --image=ghcr.io/0furkancolak/hooklane:vX.Y.Z --env=DATABASE_URL=... -- hooklane-admin bootstrap -org-name Acme -org-slug acme`;
prefer a one-off Job that reads `DATABASE_URL` from the Secret if you do not
want the URL in your shell history. Note that `ENDPOINT_SECRET_KEY` in
`secret.example.yaml` is a placeholder; never apply that file unchanged.

The API limit is off in the ConfigMap; use the Ingress annotations (or your
gateway) to rate limit per client. The manifests were rendered with
`kubectl kustomize` but not applied to a cluster by the author, so check them
against your cluster's policies (Pod Security admission, network policies).

## Upgrades and rollbacks

1. Read the CHANGELOG entry for the release.
2. Run `hooklane-admin migrate up` with the new version *before* rolling the
   new image: the `migrate` service does this on every
   `docker compose up -d` (change `HOOKLANE_VERSION` in `.env` first, or
   re-run the installer with `--version`); on Kubernetes it is the migration
   Job. Migrations are additive and the previous version keeps working
   against the new schema unless the CHANGELOG says otherwise.
3. Roll the API and the workers. Workers drain in-flight deliveries for up to
   `WORKER_SHUTDOWN_TIMEOUT` on `SIGTERM`; anything still running is
   cancelled and retried by another worker after its lease expires. With
   several replicas, roll them one or a few at a time (Kubernetes does this
   by default; the PodDisruptionBudgets keep one of each running).

`hooklane-admin migrate status` lists applied and pending migrations.

**Rollbacks.** Migrations are forward-only: there is no `down`, and an
applied migration file is never edited. To roll back, redeploy the previous
image against the already migrated database; this works whenever the
CHANGELOG did not mark the release as breaking for older versions. If a
release must be undone at the schema level, restore the pre-upgrade backup
into a new database. Take a `pg_dump` before every upgrade that adds a
migration.

## Backups

Everything Hooklane stores is in PostgreSQL, except one secret:

- **The database**: `pg_dump` it on a schedule and test restores.

  ```sh
  docker compose exec -T postgres pg_dump -U hooklane -Fc hooklane > hooklane-$(date +%F).dump
  # restore into an empty database:
  docker compose exec -T postgres pg_restore -U hooklane -d hooklane --no-owner < hooklane-YYYY-MM-DD.dump
  ```

  With a managed database use its snapshots and point-in-time recovery.
- **`ENDPOINT_SECRET_KEY`**: it encrypts endpoint signing secrets. Without
  it, a restored database is readable but every stored secret is not, and
  deliveries to those endpoints dead-letter. Keep it in a secret store apart
  from the database backups, and never change it on a running installation.
  API keys are stored as hashes and need no key; they are not recoverable.

## Configuration

Every setting is an environment variable. Invalid values stop the process at
startup with a message naming the variable.

### Common

| Variable | Default | Used by | Meaning |
| --- | --- | --- | --- |
| `DATABASE_URL` | (required) | all | PostgreSQL connection URL. Use `sslmode=require` or stricter across a network. |
| `APP_ENV` | `development` | all | `production`, `development` or `test`. `development` allows `http://` endpoints and loopback/private destinations; never use it in production. |
| `ENDPOINT_SECRET_KEY` | (required by API and worker) | api, worker | Base64 of 32 random bytes; encrypts endpoint signing secrets (AES-256-GCM). |
| `METRICS_ADDR` | (off) | api, worker | Separate listener for `/metrics`, `/healthz`, `/readyz`, e.g. `0.0.0.0:9101`. Do not expose it publicly. |
| `RETENTION_PERIOD` | `720h` | worker, admin | Delete finished events older than this; `0` disables. Minimum `1h`. |
| `RATE_LIMIT_RPS` | `50` | api | Sustained requests per second per client address on `/v1`; `0` disables. Answers `429` with `Retry-After`. Each replica keeps its own counters, and forwarded headers are not trusted, so behind a reverse proxy limit there as well. |
| `RATE_LIMIT_BURST` | `200` | api | Requests a client may send at once before `RATE_LIMIT_RPS` applies. |

### API

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port. |
| `WEB_DIR` | (off; `/app/web` in the image) | Directory with the built dashboard to serve at `/`. |

### Worker

| Variable | Default | Range | Meaning |
| --- | --- | --- | --- |
| `WORKER_CONCURRENCY` | `10` | 1–1000 | Deliveries in flight per worker. |
| `WORKER_POLL_INTERVAL` | `1s` | 10ms–1m | Wait between claims when the queue is empty. |
| `WORKER_REQUEST_TIMEOUT` | `15s` | 1s–1m | Limit for one outbound request. The lease is this plus 30s. |
| `WORKER_SHUTDOWN_TIMEOUT` | `20s` | 1s–5m | Drain time after `SIGTERM`. |
| `WORKER_MAX_ATTEMPTS` | `10` | 1–50 | Attempts before a delivery is dead-lettered. |
| `WORKER_RETRY_BASE_DELAY` | `30s` | 100ms–1h | Upper bound of the first retry delay. |
| `WORKER_RETRY_MAX_DELAY` | `4h` | 100ms–7d | Cap for any retry delay, including `Retry-After`. |

### Development only

| Variable | Meaning |
| --- | --- |
| `HOOKLANE_API_URL` | Where the dashboard's Vite dev server proxies API calls (default `http://127.0.0.1:8080`). |
| `HOOKLANE_PORT`, `HOOKLANE_APP_ENV` | Host port and `APP_ENV` for the Compose `app` profile. |
| `POSTGRES_PORT`, `POSTGRES_PASSWORD` | Compose PostgreSQL settings. |

## Network and security checklist

- Terminate TLS in front of the API; it does not redirect HTTP to HTTPS.
- Keep `METRICS_ADDR` on a private interface; it is unauthenticated.
- Give the worker outbound internet access but, where your platform allows,
  also block internal ranges at the network level. Hooklane refuses private,
  loopback, link-local (including cloud metadata) and reserved addresses on
  every connection; a firewall adds a second layer.
- Do not route webhook traffic through a proxy: the worker ignores
  `HTTP_PROXY` on purpose so the address checks see the real destination.
- Serve the dashboard only from an origin you control. It keeps the API key
  in `sessionStorage`, and the API sends a strict Content Security Policy
  with it.

## Sizing and operations

- Throughput is roughly `workers × WORKER_CONCURRENCY / average request time`.
  Watch `hooklane_queue_oldest_due_seconds`: if it keeps growing, add workers
  or concurrency.
- Each process opens a pgx pool (default size: max(4, CPUs)). Make sure
  PostgreSQL's `max_connections` covers all processes.
- Back up PostgreSQL as you would any primary database, plus
  `ENDPOINT_SECRET_KEY` (see [Backups](#backups)).
- Logs are JSON on stdout. They never contain API keys, endpoint secrets or
  payloads.

### Measured numbers

`make loadtest` (source: `scripts/loadtest/main.go`) starts a receiver in the
test process, creates an endpoint for it, ingests events and waits for the
project's deliveries to finish. It reports ingest events per second, the
latency from sending an ingest request to the first successful receiver hit
(p50/p95/p99), and checks the receiver saw every event: `missing` must be 0
and `duplicates` shows at-least-once redeliveries. The API and worker must run
with `APP_ENV=development` (set `HOOKLANE_APP_ENV=development` in the
Compose `.env`) because production refuses `http://` and private-network
destinations. Use a project of its own, since it waits for the project's
pending deliveries to reach zero. With the stack in Docker, pass
`LOADTEST_ARGS="-advertise <host address the containers can reach>"` (not
`127.0.0.1`; Docker Desktop's and OrbStack's `host.docker.internal` resolve to
addresses Hooklane blocks even in development, so use the host's LAN address).

```sh
make loadtest KEY=hlk_... PROJECT=<project id> EVENTS=2000 LOADTEST_URL=http://127.0.0.1:8090 \
  LOADTEST_ARGS="-advertise 192.168.1.42"
```

One run on a MacBook (Apple silicon, Docker via OrbStack, 2000 events, 16
ingest connections, a receiver that answers immediately), with the production
Compose file at `--scale api=2 --scale worker=2` (`WORKER_CONCURRENCY=10`),
PostgreSQL 17 in a container with its default settings, and the load generator
on the same machine. The host was heavily loaded by unrelated containers
(load average about 25), so treat these as a rough, noisy lower bound, not a
benchmark:

| Measure | Result |
| --- | --- |
| Ingest | 81 and 94 events/s in two runs of 2000 (249 events/s in a 1500-event run on a less busy moment) |
| Delivery latency (request sent to receiver hit) | p50 0.69-0.81 s, p95 1.3 s, p99 1.4-2.3 s, max 2.1-4.4 s |
| End-to-end rate | 80-91 deliveries/s |
| Missing / duplicate deliveries | 0 / 0 in both runs |
| Container CPU while loaded | api about 7%, worker 2-5%, PostgreSQL 34% of one core |

In a separate run with 10% of deliveries answered `500`, a 20 ms receiver and
one worker restarted mid-run (`docker restart`, so a graceful `SIGTERM`
drain), all 1500 events were delivered: 155 injected failures were retried,
0 events missing and 0 duplicates. The last retries took about 3 minutes
because failed deliveries wait for the retry delay.

What these numbers do not show: behaviour with a remote PostgreSQL, large
payloads, many endpoints per event, or slow receivers. Run the tool against a
staging copy of your own topology before choosing replica counts. The API
and worker containers stayed far below their CPU limits in these runs, so
the limit was elsewhere (PostgreSQL and the loaded host are the likely
candidates; this was not isolated).
