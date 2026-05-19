# Environment selector

The add-on exposes a single config option:

```yaml
options:
  environment: production
schema:
  environment: list(production|staging)
```

(`homegate/config.yaml`, verbatim.) HA renders this as a dropdown with two
choices. The agent does not read the URL endpoints directly from
`options.json` — `homegate/run.sh` translates the selected environment
into the four URL-shaped env vars the Go binary actually consumes.

## How the mapping works

`homegate/run.sh` reads `/data/options.json`, pulls `.environment` out
with `jq`, and runs a case statement. The current mapping (copied
straight from `run.sh`, not paraphrased):

```sh
case "$ENVIRONMENT" in
  staging)
    export API_BASE_URL="https://homegate-test.website/api"
    export BROKER_URL="wss://broker.homegate-test.website"
    export HOSTNAME_DOMAIN="homegate-test.website"
    export HOSTNAME_SEPARATOR="."
    ;;
  production|*)
    export API_BASE_URL="https://homegate.network/api"
    export BROKER_URL="wss://broker.homegate.network"
    export HOSTNAME_DOMAIN="homegate.network"
    export HOSTNAME_SEPARATOR="."
    ;;
esac
```

Note the `production|*` branch: anything other than `staging` (including
a missing or malformed value) falls through to production. The default
in `config.yaml` is also `production`, so a stock install is safe.

`run.sh` also sets a couple of other env vars the agent needs:

```sh
export DATA_DIR=/data
export INGRESS_PORT=8080
export HA_TARGET="${HA_TARGET:-http://homeassistant:8123}"
```

`HA_TARGET` is intentionally `${HA_TARGET:-default}` rather than an
unconditional assignment — that way the E2E compose file can override it
to `http://mock-ha:8123` and have the override stick. See
[e2e-testing.md](./e2e-testing.md) for context.

## What each var feeds

- `API_BASE_URL` — the HTTPS REST API the agent hits for
  `/device-auth/link-request`, polling, and claim.
- `BROKER_URL` — the WSS broker the agent connects to once linked.
  After the first successful claim, the agent stores the broker URL
  that the API returned in `/data/credentials.json` and prefers that —
  but if `BROKER_URL` is set in config it overrides the stored value
  (see the `cfg.BrokerURL != ""` branch in `homegate/main.go`), which
  is what makes flipping environments take effect.
- `HOSTNAME_DOMAIN` and `HOSTNAME_SEPARATOR` — used by the ingress UI
  to render the public hostname for the linked site. They don't
  affect the tunnel itself; the broker controls what hostname actually
  routes to a given device.

## Why this replaced the four-option layout

Before this change the add-on schema had four separate strings:
`API_BASE_URL`, `BROKER_URL`, `HOSTNAME_DOMAIN`, `HOSTNAME_SEPARATOR`.
The user had to set all four correctly to point at the same environment.
Problems:

- **Footgun.** Mixing `API_BASE_URL` from staging with `BROKER_URL`
  from production silently broke linking in confusing ways. The agent
  would link against one DB but try to register a tunnel against the
  other.
- **Drift.** Whenever the URL shape changed on the server side (new
  TLD, subdomain pattern, etc.) every user with a custom override
  needed to be migrated. With the dropdown, the URLs live in `run.sh`
  inside the published image and update on the next add-on upgrade.
- **Bad default ergonomics.** New users were shown four blank fields
  and had to read docs to know what to put in them. Now the default
  (`production`) just works.

The new layout trades a small loss of flexibility (you can't point at
an arbitrary URL from the HA config UI) for a much smaller surface area
of misconfiguration. For local development you can still override the
URLs by setting env vars on the container — see
[development.md](./development.md).

## Adding a new environment

Adding an environment is a two-line change plus a version bump:

1. Add a branch to the case statement in `homegate/run.sh`:

   ```sh
   case "$ENVIRONMENT" in
     dev)
       export API_BASE_URL="https://homegate-dev.example/api"
       export BROKER_URL="wss://broker.homegate-dev.example"
       export HOSTNAME_DOMAIN="homegate-dev.example"
       export HOSTNAME_SEPARATOR="."
       ;;
     staging) ...
   ```

2. Add the value to the schema in `homegate/config.yaml`:

   ```yaml
   schema:
     environment: list(production|staging|dev)
   ```

3. Bump `version:` in `config.yaml` (HA add-on store relies on this
   for the upgrade prompt to fire).

4. Update the E2E harness if you want to be able to run E2Es against
   it: add a row to the `envs` map in
   `tests/integration/harness/main.go` and a `make e2e-dev` target.

Don't forget the new environment also has to actually exist on the
server side — see the monorepo for the API/broker/hostname infra.
