#!/usr/bin/env bash
# Startet den Flash-Sale-Lasttest (flash-sale.js) im offiziellen k6-Container - keine lokale
# k6-Installation noetig, nur Docker. Parameter wie beim direkten `k6 run` (siehe README
# ## Verwendung), hier als Umgebungsvariablen:
#
#   ./run.sh                                  # Volllast gegen den Cluster (ticketflow.<dnsZone>)
#   VUS=2000 DURATION=5m ./run.sh             # noch mehr Last
#   API_URL=http://localhost:8080 VUS=50 DURATION=30s EVENT_ID=demo-konzert ./run.sh   # docker-compose
#
# Die Cluster-Adresse wird aus ticketflow.dnsZone in gitops/env/cluster.yaml abgeleitet (gleicher
# Host wie der Frontend-Ingress in charts/ticketflow-app) - nie als Literal hier eintragen.
#
# Zusaetzliche Argumente werden an `k6 run` durchgereicht, z.B. `./run.sh --quiet`.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CLUSTER_YAML="$SCRIPT_DIR/../../gitops/env/cluster.yaml"

if [[ -z "${API_URL:-}" ]]; then
  DNS_ZONE="$(sed -n 's/^[[:space:]]*dnsZone:[[:space:]]*\([^[:space:]#]*\).*/\1/p' "$CLUSTER_YAML" | head -n1)"
  if [[ -z "$DNS_ZONE" ]]; then
    echo "ticketflow.dnsZone nicht in $CLUSTER_YAML gefunden - API_URL explizit setzen." >&2
    exit 1
  fi
  API_URL="https://ticketflow.$DNS_ZONE"
fi

# Defaults auf maximale Last: alle Events (3x so viele Sitzplaetze, bevor catalog-api nur noch
# per Vorabcheck mit 409 antwortet und nichts mehr in Kafka landet) und 1000 VUs ohne Denkpause.
K6_IMAGE="${K6_IMAGE:-grafana/k6:2.3.0}"
EVENT_ID="${EVENT_ID:-all}"
VUS="${VUS:-1000}"
DURATION="${DURATION:-3m}"
HOT_SEAT_RATIO="${HOT_SEAT_RATIO:-0.2}"

network_args=()
if [[ "$(uname -s)" == "Linux" ]]; then
  # Host-Netz: der Ingress loest auf DHBWV6-Adressen (IPv6 + private 10.200.x) auf, die nur ueber
  # die Routen/das VPN des Hosts erreichbar sind - die Docker-Bridge hat kein IPv6. Deckt auch
  # localhost (docker-compose) ab.
  network_args=(--network host)
elif [[ "$API_URL" =~ ://(localhost|127\.0\.0\.1)(:|/|$) ]]; then
  # Docker Desktop (macOS/Windows): --network host erreicht den Host nicht, dort gibt es
  # stattdessen host.docker.internal.
  API_URL="${API_URL/localhost/host.docker.internal}"
  API_URL="${API_URL/127.0.0.1/host.docker.internal}"
fi

echo "k6 ($K6_IMAGE) -> $API_URL, Event $EVENT_ID, $VUS VUs fuer $DURATION"

# MSYS_NO_PATHCONV: verhindert unter Git Bash (Windows), dass /scripts umgeschrieben wird.
# --ulimit: jede VU haelt eigene Keep-Alive-Verbindungen, bei 1000+ VUs reicht das
# Container-Default fuer offene Dateien sonst nicht.
MSYS_NO_PATHCONV=1 exec sudo docker run --rm -i \
  --ulimit nofile=65536:65536 \
  "${network_args[@]}" \
  -v "$SCRIPT_DIR":/scripts:ro \
  -e API_URL="$API_URL" \
  -e EVENT_ID="$EVENT_ID" \
  -e VUS="$VUS" \
  -e DURATION="$DURATION" \
  -e HOT_SEAT_RATIO="$HOT_SEAT_RATIO" \
  "$K6_IMAGE" run /scripts/flash-sale.js "$@"
