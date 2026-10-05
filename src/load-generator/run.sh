#!/usr/bin/env bash
# Runs flash-sale.js in the k6 container (only Docker needed). Extra args go to k6 run.
#   ./run.sh                                   # full load against ticketflow.<dnsZone>
#   VUS=2000 DURATION=5m ./run.sh              # more load
#   API_URL=http://localhost:8080 VUS=50 ./run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CLUSTER_YAML="$SCRIPT_DIR/../../gitops/env/cluster.yaml"

if [[ -z "${API_URL:-}" ]]; then
  DNS_ZONE="$(sed -n 's/^[[:space:]]*dnsZone:[[:space:]]*\([^[:space:]#]*\).*/\1/p' "$CLUSTER_YAML" | head -n1)"
  if [[ -z "$DNS_ZONE" ]]; then
    echo "ticketflow.dnsZone not found in $CLUSTER_YAML - set API_URL explicitly." >&2
    exit 1
  fi
  API_URL="https://ticketflow.$DNS_ZONE"
fi

# Max load by default: all events, 1000 VUs, no think time.
K6_IMAGE="${K6_IMAGE:-grafana/k6:2.3.0}"
EVENT_ID="${EVENT_ID:-all}"
VUS="${VUS:-1000}"
DURATION="${DURATION:-3m}"
HOT_SEAT_RATIO="${HOT_SEAT_RATIO:-0.2}"

network_args=()
if [[ "$(uname -s)" == "Linux" ]]; then
  # Host network: the Ingress resolves to DHBWV6 addresses only the host can route to.
  network_args=(--network host)
elif [[ "$API_URL" =~ ://(localhost|127\.0\.0\.1)(:|/|$) ]]; then
  # Docker Desktop: --network host doesn't reach the host, use host.docker.internal.
  API_URL="${API_URL/localhost/host.docker.internal}"
  API_URL="${API_URL/127.0.0.1/host.docker.internal}"
fi

# handleSummary writes summary-<timestamp>.json here; run as the calling user so it owns the file.
RESULTS_DIR="$SCRIPT_DIR/results"
mkdir -p "$RESULTS_DIR"

echo "k6 ($K6_IMAGE) -> $API_URL, event $EVENT_ID, $VUS VUs for $DURATION"

# MSYS_NO_PATHCONV keeps Git Bash from rewriting /scripts; --ulimit for 1000+ VU connections.
MSYS_NO_PATHCONV=1 exec sudo docker run --rm -i \
  --ulimit nofile=65536:65536 \
  "${network_args[@]}" \
  --user "$(id -u):$(id -g)" \
  -v "$SCRIPT_DIR":/scripts:ro \
  -v "$RESULTS_DIR":/results \
  -e SUMMARY_DIR=/results \
  -e API_URL="$API_URL" \
  -e EVENT_ID="$EVENT_ID" \
  -e VUS="$VUS" \
  -e DURATION="$DURATION" \
  -e HOT_SEAT_RATIO="$HOT_SEAT_RATIO" \
  "$K6_IMAGE" run /scripts/flash-sale.js "$@"
