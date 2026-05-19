# Homegate add-on docs

Engineering docs for the Homegate Home Assistant add-on. The user-facing
install / quick-start lives in the [top-level README](../README.md); these
pages are for people working on the add-on itself.

## What this add-on is

A small Go binary (`homegate/main.go`) packaged as a Home Assistant OS
add-on. When installed and started inside Home Assistant:

1. It reads `/data/options.json` (populated by HA from the add-on config UI).
2. `homegate/run.sh` translates the chosen environment into the API base
   URL, broker URL, and public hostname domain, then `exec`s the agent
   binary.
3. The agent runs a small ingress panel on `:8080` (served via HA's
   ingress) for the link/claim flow.
4. Once linked, it opens a persistent WSS tunnel to the Homegate broker
   on the chosen environment.
5. The broker fronts inbound HTTPS for the site's public hostname (e.g.
   `something.homegate.network`) and the add-on proxies those requests
   to whatever `HA_TARGET` points at — by default the local Home
   Assistant on `http://homeassistant:8123`.

Net effect: a public hostname that ends at the user's local HA without
opening any inbound ports on their network.

## Repo layout

```
addon/
├── homegate/              Add-on source (Go agent, run.sh, Dockerfile, config.yaml)
│   ├── main.go            Agent entrypoint; link flow + tunnel orchestration
│   ├── internal/          tunnel, credentials, link, ui, config packages
│   ├── run.sh             Container entrypoint; maps environment -> URL env vars
│   ├── config.yaml        HA add-on manifest (slug, schema, version)
│   └── Dockerfile         Two-stage Go build on Alpine
├── tests/integration/     Docker-based E2E harness (see e2e-testing.md)
│   ├── harness/           Go program that drives the full E2E flow
│   ├── mock-ha/           Tiny HTTP server standing in for Home Assistant
│   └── compose.yml        Brings up addon + mock-ha together
├── docs/                  You are here
├── Makefile               `make e2e-stg` / `make e2e-prd`
├── README.md              User-facing install + quick start (do not refactor casually)
└── repository.yaml        HA add-on repository manifest
```

## Pages in this directory

- [development.md](./development.md) — How `config.yaml`, `run.sh`, and
  `main.go` fit together; building the image; running the container
  directly against staging without going through HA.
- [environment-selector.md](./environment-selector.md) — The
  `environment: production|staging` config option, what URLs each value
  maps to, why this replaced the previous four-option layout, and how
  to add a new environment.
- [e2e-testing.md](./e2e-testing.md) — High-level intro to the E2E
  harness under `tests/integration/`. Detailed runbook (env vars,
  prerequisites, caveats) lives in
  [tests/integration/README.md](../tests/integration/README.md).

## Things that are NOT in these docs

The add-on talks to infrastructure (API, broker, nginx, Cloudflare,
Doppler, VPS) that lives in the companion repo. For anything related to:

- Deploying or operating the API or broker
- nginx / Cloudflare config for `*.homegate.network`
- Database migrations
- Doppler config layouts
- VPS provisioning

see the monorepo `docs/` directory at
[github.com/josh-richardson/homegate/tree/master/docs](https://github.com/josh-richardson/homegate/tree/master/docs)
(notably `architecture.md` and `deployment.md`).

This repo's job is only the add-on side of the wire.

## Recently changed

A couple of things landed recently that you'll want to be aware of
when reading older PRs / issues:

- The four per-URL config options (`API_BASE_URL`, `BROKER_URL`,
  `HOSTNAME_DOMAIN`, `HOSTNAME_SEPARATOR`) were replaced with a single
  `environment: list(production|staging)` dropdown. `run.sh` now
  expands the chosen environment into the four URL env vars. See
  [environment-selector.md](./environment-selector.md).
- A Docker-based E2E harness was added under `tests/integration/`. It
  exercises the full Cloudflare → nginx → broker → tunnel → add-on
  path against a live API environment. See
  [e2e-testing.md](./e2e-testing.md).
