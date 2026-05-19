# Developing the add-on

This page is the "how do I change the add-on and try it out" reference.

## The three files that matter

The add-on is mostly three small files plus the Go source tree.

### `homegate/config.yaml`

The Home Assistant add-on manifest. Names the slug, the OCI image
pattern (`ghcr.io/josh-richardson/homegate-addon-{arch}`), the
supported architectures, ingress port, and the schema for what HA
shows in the configuration UI:

```yaml
options:
  environment: production
schema:
  environment: list(production|staging)
```

Bump `version:` whenever you change anything user-visible — HA uses
that to drive its upgrade prompt.

### `homegate/run.sh`

The container entrypoint. Three jobs:

1. Read `/data/options.json` (HA writes the user's selected options
   here) and pull out `.environment`.
2. Set the `API_BASE_URL`, `BROKER_URL`, `HOSTNAME_DOMAIN`,
   `HOSTNAME_SEPARATOR`, `DATA_DIR`, `INGRESS_PORT`, and `HA_TARGET`
   env vars for the agent. See
   [environment-selector.md](./environment-selector.md) for the case
   statement and what each var feeds.
3. `exec agent` (replaces the shell with the Go binary so it gets
   PID 1 and signals reach it correctly).

### `homegate/main.go`

The actual agent. On startup it:

- Loads config from env (the vars `run.sh` exported).
- Looks for stored credentials in `/data/credentials.json`. If
  present, opens a tunnel against the stored broker URL and serves
  the ingress UI.
- If not present, runs `startLinkFlow`: POSTs `/device-auth/link-request`
  against `API_BASE_URL`, writes `/data/link-request.json`, surfaces
  the verification URL via the UI, and polls
  `/device-auth/link-status` until the user confirms in the
  dashboard. On `completed`, it claims a JWT + broker URL, saves
  credentials, and opens the tunnel.

The tunnel itself lives under `internal/tunnel/` and isn't covered
here; for local work you mostly only need to know that it dials
`BROKER_URL` over WSS with the device JWT and then proxies inbound
HTTP requests to `HA_TARGET`.

## Building the image

The standard way is to let HA's add-on builder build it from the
multi-arch base. For local development you can build it directly with
docker:

```sh
cd homegate
docker build -t homegate-addon:dev .
```

The Dockerfile is a two-stage Go build on Alpine:

```dockerfile
FROM golang:1-alpine AS builder
...
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X main.version=$(grep '^version:' /tmp/config.yaml | tr -d ' \"' | cut -d: -f2)" \
    -o /agent .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates jq
COPY --from=builder /agent /usr/local/bin/agent
COPY run.sh /usr/local/bin/run.sh
RUN chmod +x /usr/local/bin/run.sh
EXPOSE 8080
ENTRYPOINT ["run.sh"]
```

Two things to note:

- The `version` string is baked in at build time from `config.yaml`,
  so a `docker build` without `config.yaml` will produce a version of
  `dev`. That's fine for local work.
- `jq` is installed in the runtime image specifically for `run.sh` to
  parse `/data/options.json`.

## Running the container against staging

The fastest dev loop is to skip Home Assistant entirely and run the
add-on container directly with a mounted `/data` dir. The agent only
needs `/data/options.json` to exist; HA's job is essentially just to
write that file.

```sh
mkdir -p /tmp/homegate-dev-data
cat > /tmp/homegate-dev-data/options.json <<'JSON'
{ "environment": "staging" }
JSON

docker run --rm -it \
  -p 8080:8080 \
  -v /tmp/homegate-dev-data:/data \
  homegate-addon:dev
```

Open `http://localhost:8080/` and you'll get the ingress UI, complete
with a verification URL pointing at
`https://homegate-test.website/auth/...`. Visit it as a logged-in stg
user, confirm the link, and the container will claim credentials and
connect to the staging broker.

After confirmation, `/tmp/homegate-dev-data/credentials.json` will
exist on the host. Delete it (or `rm -rf` the whole directory) to
re-run the link flow on the next container start.

### Overriding HA_TARGET

By default `run.sh` sets `HA_TARGET=http://homeassistant:8123`, which
won't resolve outside HA's network. To point the add-on at something
reachable (a local mock, a real HA on your LAN, etc.), pass it
explicitly:

```sh
docker run --rm -it \
  -p 8080:8080 \
  -e HA_TARGET=http://host.docker.internal:8123 \
  -v /tmp/homegate-dev-data:/data \
  homegate-addon:dev
```

`run.sh` uses `${HA_TARGET:-default}` so the env var wins. The E2E
harness relies on this same behaviour to route to its `mock-ha`
sidecar — see [e2e-testing.md](./e2e-testing.md).

### Overriding endpoints without rebuilding

If you need to point at endpoints that aren't `production` or
`staging` (e.g. a local API on your laptop), the simplest path is to
set the four URL env vars on the `docker run` command. The agent
reads them directly; `run.sh`'s case statement only sets them when
they aren't already in the environment-derived form, but since
`run.sh` does an unconditional `export`, the cleanest override is to
bypass `run.sh` entirely:

```sh
docker run --rm -it \
  -p 8080:8080 \
  -e API_BASE_URL=http://host.docker.internal:3000/api \
  -e BROKER_URL=ws://host.docker.internal:4000 \
  -e HOSTNAME_DOMAIN=localhost \
  -e HOSTNAME_SEPARATOR=- \
  -e DATA_DIR=/data \
  -e INGRESS_PORT=8080 \
  -e HA_TARGET=http://host.docker.internal:8123 \
  -v /tmp/homegate-dev-data:/data \
  --entrypoint agent \
  homegate-addon:dev
```

This skips `run.sh` and runs the agent binary directly with the env
vars the agent's `config.Load()` expects. Useful for full local-stack
debugging against an API/broker running on your laptop.

## Iterating on the Go code

For pure code changes, `go build` / `go test ./...` in `homegate/`
is faster than rebuilding the image. The `internal/` packages are
unit-tested.

When you need to test the run.sh wiring or anything env-related,
rebuild the image and use the staging dev loop above. When you need
to test the full wire (broker, nginx, Cloudflare, real DNS), use the
E2E harness — see [e2e-testing.md](./e2e-testing.md).

## Things to remember

- `config.yaml` `version:` bumps are what makes HA prompt users to
  upgrade. If you ship a fix and forget the bump, nobody picks it up.
- `/data` is the only writable, persistent path the agent has. Three
  files live there: `options.json` (HA-written), `link-request.json`
  (agent-written, during link flow), `credentials.json`
  (agent-written, after successful claim).
- The agent writes those as root inside the container. If you
  bind-mount `/data` for local dev, don't be surprised when host-side
  reads need `sudo` (or use `docker compose exec` like the harness
  does).
