{{/*
Base helpers plus per-component variants (catalog-api, frontend) taking a dict with root and
component.
*/}}
{{- define "ticketflow-app.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "ticketflow-app.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "ticketflow-app.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "ticketflow-app.labels" -}}
helm.sh/chart: {{ include "ticketflow-app.chart" . }}
{{ include "ticketflow-app.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "ticketflow-app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ticketflow-app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* --- Per-component variants (dict with root and component) --- */}}

{{- define "ticketflow-app.componentName" -}}
{{- printf "%s-%s" (include "ticketflow-app.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "ticketflow-app.componentSelectorLabels" -}}
{{ include "ticketflow-app.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "ticketflow-app.componentLabels" -}}
{{ include "ticketflow-app.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Image: registry and tag from gitops/env/cluster.yaml; image.* only as an override. */}}
{{- define "ticketflow-app.componentImage" -}}
{{- $repo := .image.repository | default (printf "%s/ticketflow-%s" (required "ticketflow.imageRegistry is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .root.Values.ticketflow.imageRegistry) .component) -}}
{{- $tag := .image.tag | default (required "ticketflow.imageTag is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .root.Values.ticketflow.imageTag) -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{/* ticketflow.<dnsZone> unless frontend.ingress.host is set. */}}
{{- define "ticketflow-app.ingressHost" -}}
{{- .Values.frontend.ingress.host | default (printf "ticketflow.%s" (required "ticketflow.dnsZone is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.dnsZone)) -}}
{{- end -}}

{{/* Kafka bootstrap (SASL listener 9092). */}}
{{- define "ticketflow-app.kafkaBootstrap" -}}
{{- .Values.catalogApi.config.kafkaBrokers | default (printf "%s-kafka-bootstrap.%s.svc:9092" (required "ticketflow.kafka.clusterName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}

{{/* Apicurio registry URL. */}}
{{- define "ticketflow-app.schemaRegistryUrl" -}}
{{- .Values.catalogApi.config.registryUrl | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
{{/* Postgres in the same namespace; read-only service (replicas), catalog-api never writes. */}}
{{- define "ticketflow-app.databaseHost" -}}
{{- .Values.catalogApi.database.host | default (printf "%s-postgres-cluster-ro" (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}

{{- define "ticketflow-app.databaseName" -}}
{{- .Values.catalogApi.database.name | default (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName) -}}
{{- end -}}

{{/* CNPG-generated secret (keys username/password). */}}
{{- define "ticketflow-app.databaseSecretName" -}}
{{- .Values.catalogApi.database.credentialsSecretName | default (printf "%s-postgres-cluster-app" (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}
