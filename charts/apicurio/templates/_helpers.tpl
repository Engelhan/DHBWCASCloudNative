{{- define "apicurio.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "apicurio.fullname" -}}
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

{{- define "apicurio.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "apicurio.labels" -}}
helm.sh/chart: {{ include "apicurio.chart" . }}
{{ include "apicurio.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "apicurio.selectorLabels" -}}
app.kubernetes.io/name: {{ include "apicurio.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Bootstrap-Adresse fuer den kafkasql-Storage. Port 9093 ist der "unauth"-Listener (siehe
values.yaml) - Apicurio 3.3.3 kann kein SCRAM/PLAIN-SASL. Name und Namespace kommen aus
gitops/env/cluster.yaml; Apicurio laeuft ohnehin im selben Namespace wie der Kafka-Cluster, der
Eintrag bleibt dadurch auch bei einer Namespace-Umbenennung korrekt.
*/}}
{{- define "apicurio.kafkaBootstrap" -}}
{{- .Values.config.kafkaBootstrapServers | default (printf "%s-kafka-bootstrap.%s.svc:9093" (required "ticketflow.kafka.clusterName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
