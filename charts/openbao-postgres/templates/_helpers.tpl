{{/*
Derived from gitops/env/cluster.yaml (ticketflow.openbao.databaseName). Separate CNPG cluster:
OpenBao's storage backend.
*/}}
{{- define "openbao-postgres.databaseName" -}}
{{- required "ticketflow.openbao.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.openbao.databaseName -}}
{{- end -}}

{{- define "openbao-postgres.clusterName" -}}
{{- printf "%s-postgres-cluster" (include "openbao-postgres.databaseName" .) -}}
{{- end -}}

{{/* CNPG-generated app secret (<clusterName>-app), used directly by charts/openbao. */}}
{{- define "openbao-postgres.appSecretName" -}}
{{- printf "%s-app" (include "openbao-postgres.clusterName" .) -}}
{{- end -}}
