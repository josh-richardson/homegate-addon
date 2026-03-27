#!/usr/bin/env sh
set -e

# Read HA addon options and export as env vars for the agent
CONFIG_PATH=/data/options.json

if [ -f "$CONFIG_PATH" ]; then
  export API_BASE_URL=$(jq -r '.api_base_url' "$CONFIG_PATH")
  export BROKER_URL=$(jq -r '.broker_url' "$CONFIG_PATH")
  export HOSTNAME_DOMAIN=$(jq -r '.hostname_domain' "$CONFIG_PATH")
fi

export DATA_DIR=/data
export INGRESS_PORT=8080

exec agent
