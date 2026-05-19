#!/usr/bin/env sh
set -e

CONFIG_PATH=/data/options.json
ENVIRONMENT=production

if [ -f "$CONFIG_PATH" ]; then
  ENVIRONMENT=$(jq -r '.environment // "production"' "$CONFIG_PATH")
fi

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

export DATA_DIR=/data
export INGRESS_PORT=8080
export HA_TARGET=http://homeassistant:8123

exec agent
