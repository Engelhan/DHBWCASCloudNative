{{/*
Standard-helm-create-Helper - identisch zu charts/reservation-worker und charts/ticketflow-app,
nur mit anderem Chart-Namen. Bewusst je Chart dupliziert statt als Library-Chart ausgelagert:
eine file://-Dependency mehr pro Chart waere fuer drei kurze Defines mehr Aufwand als Nutzen.
*/}}

{{/*
Vollstaendiger Image-Name. Registry und Tag kommen zentral aus gitops/env/cluster.yaml
(ticketflow.imageRegistry / .imageTag), damit ein Image-Bump nur EINE Stelle im Repo beruehrt -
.github/workflows/build-images.yml schreibt den Tag dort automatisch fort. image.repository bzw.
image.tag in values.yaml bleiben als bewusster Einzelfall-Override moeglich (z.B. ein Test-Build
aus einer privaten Registry), sind aber normalerweise leer.
*/}}
{{- define "notification-worker.image" -}}
{{- $repo := .Values.image.repository | default (printf "%s/ticketflow-notification-worker" (required "ticketflow.imageRegistry fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageRegistry)) -}}
{{- $tag := .Values.image.tag | default (required "ticketflow.imageTag fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageTag) -}}
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
{{/*
Bootstrap-Adresse des Kafka-Clusters. Kafka laeuft in einem eigenen Namespace, getrennt von der
Anwendung - Name und Namespace kommen deshalb aus gitops/env/cluster.yaml statt als
ausgeschriebener DNS-Name hier in values.yaml zu stehen (er stand frueher in vier Charts parallel).
Port 9092 ist Strimzis "plain"-Listener (SASL_PLAINTEXT seit B2). config.kafkaBrokers ueberschreibt die Ableitung, falls noetig.
*/}}
{{- define "notification-worker.kafkaBootstrap" -}}
{{- .Values.config.kafkaBrokers | default (printf "%s-kafka-bootstrap.%s.svc:9092" (required "ticketflow.kafka.clusterName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}

{{/*
URL der Apicurio-Registry - laeuft im selben Kafka-Namespace.
*/}}
{{- define "notification-worker.schemaRegistryUrl" -}}
{{- .Values.config.registryUrl | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
