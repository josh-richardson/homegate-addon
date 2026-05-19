# E2E testing

The add-on ships with a Docker-based end-to-end harness under
`tests/integration/`. It drives the entire production path of the
system — signup, site creation, device linking, tunnel establishment,
and a real HTTPS round-trip through Cloudflare and the broker — against
a live API environment (staging by default).

If unit tests answer "does this function do what it says?", the E2E
harness answers "does the wire still go from end to end?".

## What it tests

A single `make e2e-stg` run, in order:

1. Signs up a fresh throwaway user against the staging API.
2. Pulls the confirmation email out of the Mailtrap sandbox inbox and
   confirms the account, then logs in.
3. Creates a site (gets a generated public hostname like
   `something.homegate-test.website`).
4. `docker compose up` brings up two containers:
   - the **add-on** built from `homegate/Dockerfile`, with
     `environment: staging` in a mounted `options.json` and
     `HA_TARGET=http://mock-ha:8123` set via compose env.
   - **mock-ha**, a tiny Go HTTP server that returns a known body on
     every request. Stands in for the real Home Assistant the add-on
     would normally proxy to.
5. Reads the add-on's `/data/link-request.json` (via
   `docker compose exec addon cat …` — see "Why exec, not host read"
   below) to learn the link request ID and claim secret.
6. Confirms the link as the logged-in user. The add-on claims its
   credentials, connects to the broker, and the broker marks the site
   online.
7. Performs `GET https://<hostname>/` from the host. The request
   traverses **Cloudflare → VPS nginx → broker → tunnel → add-on →
   mock-ha**. The harness asserts the response body is the known
   mock-ha body.
8. Tears the site down and `docker compose down -v`.

If any step fails the run exits non-zero with a `step <name>: <err>`
line.

## When to run it

- Before tagging a new add-on release.
- After any change to `homegate/run.sh`, `homegate/main.go`,
  `homegate/config.yaml`, or `homegate/Dockerfile`.
- After changes on the server side (broker, nginx, API) that touch the
  tunnel path. The harness is the cheapest way to confirm both sides
  still agree on the wire format.

It's not currently part of CI — runs depend on Doppler creds for the
Mailtrap sandbox and against the live stg API — so it's a manual
gate today.

## How to run it

From the addon repo root:

```sh
make e2e-stg
```

That just shells out to:

```sh
cd tests/integration/harness && E2E_ENV=staging go run .
```

There's also `make e2e-prd`, which runs against production. It creates
real records in the production DB; use it sparingly (release smoke
test, post-incident verification) and never in a loop.

You'll need a handful of env vars set before either target works
(Mailtrap credentials, a test password, a signup domain). The full
list and the Doppler config that provides them is documented in:

- [tests/integration/README.md](../tests/integration/README.md) — full
  runbook: env vars, prerequisites, optional tuning knobs, caveats.

The intent is that this page stays at the "what / when / why" level
and the detailed runbook stays next to the code in
`tests/integration/`.

## How it's wired

- `tests/integration/compose.yml` — two services (`addon`, `mock-ha`).
  The add-on bind-mounts `./data:/data` so the harness can drop an
  `options.json` in there before bringing the stack up.
- `tests/integration/harness/main.go` — the orchestrator. Step list
  lives in `main()`.
- `tests/integration/mock-ha/main.go` — the HA stand-in. Returns
  `MOCK_HA_BODY` (default `homegate-e2e-ok`) on every path.

## Why exec, not host read

The add-on container runs as root and writes
`/data/link-request.json` as root. The harness used to read that file
from the host via the bind-mounted `./data` directory, but on hosts
where the user isn't in a group that can read root-owned files inside
the bind mount it would fail with EACCES.

The harness now reads via `docker compose exec addon cat
/data/link-request.json`, which goes through the container's perms
instead. This is the simplest fix and avoids needing `chmod`/`chown`
in `run.sh` just to satisfy the test harness.

## Why HA_TARGET is conditional in run.sh

Related: `run.sh` used to unconditionally set
`HA_TARGET=http://homeassistant:8123`. The compose file in
`tests/integration/` sets `HA_TARGET=http://mock-ha:8123` in the
service `environment:` block, but because `run.sh` then overwrote it,
the add-on inside the harness silently proxied to the real
`homeassistant` DNS name (which doesn't exist in the compose network)
instead of mock-ha. The fix is:

```sh
export HA_TARGET="${HA_TARGET:-http://homeassistant:8123}"
```

so that compose / docker env vars win when set. This is the same
pattern HA itself uses for its overrides, so the production behavior
(no env var set, default to `homeassistant:8123`) is unchanged.

## Cleanup

The harness deletes its site on exit but does **not** delete the
throwaway user (`e2e-<ts>@<E2E_SIGNUP_DOMAIN>`). After a few hundred
runs the stg `users` table will look messy. Either purge periodically
via admin tooling or live with it.
