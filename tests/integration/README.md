# Homegate addon E2E harness

Spins up the addon container against a live API (staging or production) and
exercises signup → site create → link/claim → tunnelled HTTP round-trip end
to end. Catches breakage that unit tests in either repo can't.

## What it does

1. Generates a fresh `e2e-<ts>@<E2E_SIGNUP_DOMAIN>` email and POSTs `/auth/register`.
2. Polls the Mailtrap sandbox inbox for the confirmation email and extracts the token.
3. Confirms the account and logs in (keeps the session cookie).
4. Creates a site (gets back a generated hostname like `foo-bar.homegate.network`).
5. `docker compose up` brings up the addon container plus a `mock-ha` sidecar.
6. Reads `tests/integration/data/link-request.json` (bind-mounted from the addon)
   to learn the `requestId`.
7. POSTs `/device-auth/link-confirm` as the logged-in user. The addon then
   claims credentials and connects to the broker.
8. Polls `/sites/{id}` until `isOnline=true`.
9. Performs `GET https://<hostname>/` from the host. The request traverses
   Cloudflare → VPS nginx → broker → tunnel → addon → mock-ha. Asserts the
   mock-ha body is returned.
10. Tears the site down and `docker compose down -v`.

## Prerequisites

The API on stg needs `MAILTRAP_USE_SANDBOX=true` and `MAILTRAP_INBOX_ID=<id>`
set in Doppler so signup emails route to the sandbox inbox instead of being
delivered to real recipients. See `monorepo/CLAUDE.md` and `apps/api/src/email/email.service.ts`.

## Running

From the addon repo root:

```sh
make e2e-stg   # tests against homegate-test.website
make e2e-prd   # tests against homegate.network (careful: real prod)
```

## Required env vars

| Var                    | Purpose                                              |
| ---------------------- | ---------------------------------------------------- |
| `E2E_PASSWORD`         | Password for the throwaway test user                 |
| `MAILTRAP_API_TOKEN`   | Mailtrap account API token with inbox-read scope     |
| `MAILTRAP_ACCOUNT_ID`  | Mailtrap account id                                  |
| `MAILTRAP_INBOX_ID`    | Sandbox inbox id that receives signup confirmations  |
| `E2E_SIGNUP_DOMAIN`    | Domain part for `e2e-<ts>@<domain>` addresses        |

These typically come from Doppler `homegate/test`. Wrap the make target with
`doppler run --project homegate --config test --` once that config exists.

## Optional env vars

| Var                  | Default                  | Purpose                                       |
| -------------------- | ------------------------ | --------------------------------------------- |
| `E2E_CAPTCHA_TOKEN`  | (none)                   | Turnstile token if captcha is enforced        |
| `E2E_COMPOSE_FILE`   | `../compose.yml`         | Path to docker compose file                   |
| `E2E_DATA_DIR`       | `../data`                | Host path bind-mounted as the addon's `/data` |
| `E2E_MOCK_HA_BODY`   | `homegate-e2e-ok`        | Expected body from the mock-ha sidecar        |
| `E2E_LINK_TIMEOUT`   | `30`                     | Seconds to wait for the addon link-request   |
| `E2E_TUNNEL_TIMEOUT` | `60`                     | Seconds to wait for device to come online    |

## Caveats

- The throwaway user is not cleaned up automatically; only the site is. Build
  up periodically and either purge via admin tooling or live with it.
- If Turnstile is enforced on stg, the harness needs a passing captcha token.
  Use a Turnstile "always-pass" test site key on stg to avoid this.
- `make e2e-prd` will create real records in the production DB. Run sparingly.
