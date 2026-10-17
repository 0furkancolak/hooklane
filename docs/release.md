# Releasing

Versions follow Semantic Versioning and are tagged `vMAJOR.MINOR.PATCH`.
Pushing a tag runs [.github/workflows/release.yaml](../.github/workflows/release.yaml),
which:

- builds `hooklane-api`, `hooklane-worker` and `hooklane-admin` for linux and
  darwin (amd64, arm64) with the version embedded (`hooklane-admin version`)
- builds the dashboard archive
- pushes `ghcr.io/<owner>/hooklane:<tag>` and `:latest` for amd64 and arm64
- creates a GitHub release with the archives, `SHA256SUMS` and the matching
  CHANGELOG section as notes (the job fails if that section is missing)

## Checklist

1. `main` is green in CI.
2. Move the `Unreleased` entries in `CHANGELOG.md` under a new
   `## [X.Y.Z] - YYYY-MM-DD` heading, call out breaking changes and new
   migrations, and update the comparison links at the bottom.
3. Check that the README, `docs/deployment.md` and `openapi.yaml` describe the
   release (new variables, routes, defaults).
4. Run the clean-install check below on a machine without a previous
   checkout or volume.
5. Merge the changelog commit, then tag and push from `main`:
   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z
   ```
6. When the workflow finishes, check the release page, pull the image and run
   `docker run --rm ghcr.io/<owner>/hooklane:vX.Y.Z hooklane-admin version`.
7. The first time, make the GHCR package public if the image should be
   pullable without logging in (package settings on GitHub).

## Clean-install check

From an empty directory, follow the README's Quick start exactly. To avoid
clashing with an existing development stack, give the check its own Compose
project and ports:

```sh
export COMPOSE_PROJECT_NAME=hooklane-check POSTGRES_PORT=5544 HOOKLANE_PORT=8190
```

For a local receiver, add `HOOKLANE_APP_ENV=development` to `.env` and run
the example receiver on the Compose network so the worker can reach it:

```sh
docker run -d --rm --name hooklane-check-receiver \
  --network hooklane-check_default --network-alias receiver \
  -e HOOKLANE_SECRET=<endpoint secret> -e HOST=0.0.0.0 \
  -v "$PWD/examples/receiver-node:/app:ro" node:24-alpine node /app/server.mjs
```

and register `http://receiver:9000/webhooks` as the endpoint.

Pass when: `docker compose --profile app up -d --build` brings up postgres,
runs `migrate` to completion and starts `api` and `worker`;
`curl localhost:8190/readyz` returns `ready`; bootstrap prints a key; the
dashboard signs in; an event is delivered and the receiver logs it (a
receiver started with a different secret answers `401`). Afterwards,
`docker compose --profile app down -v` removes the check's containers and
its database volume.
