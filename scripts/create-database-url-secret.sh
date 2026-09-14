#!/usr/bin/env bash
# Erstellt/aktualisiert das Secret "ticketflow-database-url" (Namespace "ticketflow", Key
# DATABASE_URL) aus dem CNPG-Cluster von charts/postgres-cluster (Namespace "application").
#
# Grund: catalog-api/reservation-worker erwarten laut README einen fertigen Connection-String
# als Secret, charts/postgres-cluster liefert aber nur ein Secret mit getrennten
# username/password-Feldern (siehe charts/ticketflow/README.md ## Bekannte offene Abhaengigkeit).
# Dieses Skript ueberbrueckt die Luecke, bis das im Chart selbst geloest ist.
#
# Voraussetzung: KUBECONFIG zeigt auf den Zielcluster, charts/postgres-cluster ist bereits
# per ArgoCD synced (sonst gibt's das Secret/den Cluster noch nicht).

set -euo pipefail

NS_DB="application"
NS_APP="ticketflow"

SECRET_NAME=$(kubectl -n "$NS_DB" get secret -o name | grep postgres-user | head -1 | cut -d/ -f2 || true)
if [ -z "$SECRET_NAME" ]; then
  echo "Kein *-postgres-user Secret in Namespace $NS_DB gefunden. Ist charts/postgres-cluster schon synced? (kubectl -n $NS_DB get applications.argoproj.io -A)" >&2
  exit 1
fi

CLUSTER_NAME=$(kubectl -n "$NS_DB" get cluster.postgresql.cnpg.io -o name | head -1 | cut -d/ -f2 || true)
if [ -z "$CLUSTER_NAME" ]; then
  echo "Kein CNPG Cluster (cluster.postgresql.cnpg.io) in Namespace $NS_DB gefunden." >&2
  exit 1
fi

USERNAME=$(kubectl -n "$NS_DB" get secret "$SECRET_NAME" -o jsonpath='{.data.username}' | base64 -d)
PASSWORD=$(kubectl -n "$NS_DB" get secret "$SECRET_NAME" -o jsonpath='{.data.password}' | base64 -d)

# CNPG legt fuer jeden Cluster automatisch einen "<cluster-name>-rw"-Service an (Primary,
# read-write). DB-Name aktuell "postgres" (charts/postgres-cluster/values.yaml database.name
# Default) - noch nicht auf "ticketflow" umbenannt, siehe offene Team-Abstimmung dazu.
HOST="${CLUSTER_NAME}-rw.${NS_DB}.svc.cluster.local"
DBNAME="${1:-postgres}"

DATABASE_URL="postgres://${USERNAME}:${PASSWORD}@${HOST}:5432/${DBNAME}?sslmode=require"

kubectl -n "$NS_APP" create secret generic ticketflow-database-url \
  --from-literal=DATABASE_URL="$DATABASE_URL" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret ticketflow-database-url in Namespace $NS_APP gesetzt/aktualisiert (Host: $HOST, DB: $DBNAME)."
