{{/* Standard helpers (duplicated per chart on purpose). */}}

{{/* Image: registry and tag from gitops/env/cluster.yaml; image.* only as an override. */}}
{{- define "notification-worker.image" -}}
{{- $repo := .Values.image.repository | default (printf "%s/ticketflow-notification-worker" (required "ticketflow.imageRegistry is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageRegistry)) -}}
{{- $tag := .Values.image.tag | default (required "ticketflow.imageTag is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageTag) -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "notification-worker.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "notification-worker.fullname" -}}
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

{{/*
Chart-owned secret (SMTP, Kafka mTLS). The suffix avoids a clash with the Strimzi secret of the
same-named KafkaUser. secret.name overrides it.
*/}}
{{- define "notification-worker.secretName" -}}
{{- .Values.secret.name | default (printf "%s-credentials" (include "notification-worker.fullname" .)) -}}
{{- end -}}

{{- define "notification-worker.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "notification-worker.labels" -}}
helm.sh/chart: {{ include "notification-worker.chart" . }}
{{ include "notification-worker.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "notification-worker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "notification-worker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
{{/* Kafka bootstrap (SASL listener 9092); config.kafkaBrokers overrides it. */}}
{{- define "notification-worker.kafkaBootstrap" -}}
{{- .Values.config.kafkaBrokers | default (printf "%s-kafka-bootstrap.%s.svc:9092" (required "ticketflow.kafka.clusterName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}

{{/* Apicurio registry URL. */}}
{{- define "notification-worker.schemaRegistryUrl" -}}
{{- .Values.config.registryUrl | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
