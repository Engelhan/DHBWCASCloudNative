{{/*
Zentrale Ableitungen aus gitops/env/cluster.yaml.

Fast identisch zu charts/postgres-cluster/templates/_helpers.tpl, nur mit
ticketflow.openbao.databaseName statt ticketflow.postgres.databaseName - eigener Schluessel, weil
dies eine zweite, getrennte CNPG-Cluster-CR ist (OpenBaos eigenes Storage-Backend, nicht die
TicketFlow-App-DB).
*/}}
{{- define "openbao-postgres.databaseName" -}}
{{- required "ticketflow.openbao.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.openbao.databaseName -}}
{{- end -}}

{{/* Name der CNPG-Cluster-CR. */}}
{{- define "openbao-postgres.clusterName" -}}
{{- printf "%s-postgres-cluster" (include "openbao-postgres.databaseName" .) -}}
{{- end -}}

{{/*
Anders als charts/postgres-cluster gibt es hier KEIN "openbao-postgres.userSecretName" - die
Cluster-CR (postgres-cluster.yaml) laesst bootstrap.initdb.secret bewusst weg, damit CNPG das
App-User-Credential selbst generiert (Secret "<clusterName>-app", Keys username/password/host/
port/dbname/uri/jdbc-uri). charts/openbao referenziert dieses generierte Secret direkt statt eines
hier chart-eigenen - siehe dessen values.yaml (extraSecretEnvironmentVars/BAO_PG_CONNECTION_URL).
*/}}
{{- define "openbao-postgres.appSecretName" -}}
{{- printf "%s-app" (include "openbao-postgres.clusterName" .) -}}
{{- end -}}

{{/* Namespace dieses Charts (= der ArgoCD-Application), fuer die NetworkPolicy-Ingress-Regel. */}}
{{- define "openbao-postgres.namespace" -}}
{{- required "ticketflow.namespaces.openbao fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.openbao -}}
{{- end -}}
