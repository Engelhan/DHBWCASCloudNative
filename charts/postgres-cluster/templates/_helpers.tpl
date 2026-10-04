{{/*
Derived from gitops/env/cluster.yaml (ticketflow.postgres.databaseName, shared with the apps).
*/}}
{{- define "postgres-cluster.databaseName" -}}
{{- required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName -}}
{{- end -}}

{{- define "postgres-cluster.clusterName" -}}
{{- printf "%s-postgres-cluster" (include "postgres-cluster.databaseName" .) -}}
{{- end -}}

{{/*
CNPG-generated app secret (<clusterName>-app); read directly by catalog-api/reservation-worker.
*/}}
{{- define "postgres-cluster.appSecretName" -}}
{{- printf "%s-app" (include "postgres-cluster.clusterName" .) -}}
{{- end -}}
