# Homegate Home Assistant Add-on

Secure remote access to Home Assistant via managed WebSocket tunnels.

## Installation

1. In Home Assistant, go to **Settings → Add-ons → Add-on Store**
2. Click the three-dot menu → **Repositories**
3. Add: `https://github.com/josh-richardson/homegate-addon`
4. Find "Homegate" in the list and install

## Usage

1. Sign up at [homegate.network](https://homegate.network)
2. Create a site and generate a claim token
3. Open the Homegate add-on panel in Home Assistant
4. Enter the claim token to link your instance

## For contributors

Engineering documentation lives in [`docs/`](docs/README.md):

- [`docs/environment-selector.md`](docs/environment-selector.md) — the `environment: production|staging` config option
- [`docs/e2e-testing.md`](docs/e2e-testing.md) — running the Docker-based E2E harness
- [`docs/development.md`](docs/development.md) — local build, test, and debug

For server-side infrastructure (API, web, broker, deployment, secrets),
see the [homegate monorepo](https://github.com/josh-richardson/homegate).

## License

MIT
