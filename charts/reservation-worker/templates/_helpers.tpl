{{/* Image: registry and tag from gitops/env/cluster.yaml; image.* only as an override. */}}
{{- define "reservation-worker.image" -}}
{{- $repo := .Values.image.repository | default (printf "%s/ticketflow-reservation-worker" (required "ticketflow.imageRegistry is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageRegistry)) -}}
{{- $tag := .Values.image.tag | default (required "ticketflow.imageTag is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageTag) -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "reservation-worker.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "reservation-worker.fullname" -}}
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

{{- define "reservation-worker.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "reservation-worker.labels" -}}
helm.sh/chart: {{ include "reservation-worker.chart" . }}
{{ include "reservation-worker.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "reservation-worker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "reservation-worker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
{{/* Kafka bootstrap (SASL listener 9092); config.kafkaBrokers overrides it. */}}
{{- define "reservation-worker.kafkaBootstrap" -}}
{{- .Values.config.kafkaBrokers | default (printf "%s-kafka-bootstrap.%s.svc:9092" (required "ticketflow.kafka.clusterName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}

{{/* Apicurio registry URL. */}}
{{- define "reservation-worker.schemaRegistryUrl" -}}
{{- .Values.config.registryUrl | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
{{/* Postgres in the same namespace; host derived from ticketflow.postgres.databaseName. */}}
{{- define "reservation-worker.databaseHost" -}}
{{- .Values.database.host | default (printf "%s-postgres-cluster-rw" (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}

{{- define "reservation-worker.databaseName" -}}
{{- .Values.database.name | default (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName) -}}
{{- end -}}

{{/* CNPG-generated secret (keys username/password). */}}
{{- define "reservation-worker.databaseSecretName" -}}
{{- .Values.database.credentialsSecretName | default (printf "%s-postgres-cluster-app" (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}
