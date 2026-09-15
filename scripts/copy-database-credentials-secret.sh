#!/usr/bin/env bash
# Kopiert das Postgres-Credentials-Secret (Keys "username"/"password") von
# charts/postgres-cluster (Namespace "application") unveraendert in den "ticketflow"-Namespace,
# damit catalog-api es per secretKeyRef referenzieren kann - Kubernetes erlaubt kein
# secretKeyRef ueber Namespace-Grenzen hinweg.
#
# Team-Entscheidung (siehe charts/ticketflow/values.yaml catalogApi.database-Kommentar):
# die Anwendung baut sich den Connection-String selbst aus Host/Port/Name (ConfigMap) +
# Username/Passwort (dieses Secret) zusammen - deshalb wird hier nichts umformatiert, nur
# kopiert.
#
# Voraussetzung: KUBECONFIG zeigt auf den Zielcluster, charts/postgres-cluster ist bereits
# per ArgoCD synced (sonst gibt's das Secret noch nicht).

set -euo pipefail

NS_DB="application"
NS_APP="ticketflow"
TARGET_SECRET="ticketflow-postgres-credentials"

SECRET_NAME=$(kubectl -n "$NS_DB" get secret -o name | grep postgres-user | head -1 | cut -d/ -f2 || true)
if [ -z "$SECRET_NAME" ]; then
  echo "Kein *-postgres-user Secret in Namespace $NS_DB gefunden. Ist charts/postgres-cluster schon synced? (kubectl -n $NS_DB get applications.argoproj.io -A)" >&2
  exit 1
fi

USERNAME=$(kubectl -n "$NS_DB" get secret "$SECRET_NAME" -o jsonpath='{.data.username}' | base64 -d)
PASSWORD=$(kubectl -n "$NS_DB" get secret "$SECRET_NAME" -o jsonpath='{.data.password}' | base64 -d)

kubectl -n "$NS_APP" create secret generic "$TARGET_SECRET" \
  --from-literal=username="$USERNAME" \
  --from-literal=password="$PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Secret $TARGET_SECRET in Namespace $NS_APP angelegt/aktualisiert (kopiert aus $NS_DB/$SECRET_NAME)."
echo "Hinweis: falls sich Host/DB-Name in charts/postgres-cluster aendern, catalogApi.database in charts/ticketflow/values.yaml entsprechend anpassen."
