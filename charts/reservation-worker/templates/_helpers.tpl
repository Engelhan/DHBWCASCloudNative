{{/*
Vollstaendiger Image-Name. Registry und Tag kommen zentral aus gitops/env/cluster.yaml
(ticketflow.imageRegistry / .imageTag), damit ein Image-Bump nur EINE Stelle im Repo beruehrt -
.github/workflows/build-images.yml schreibt den Tag dort automatisch fort. image.repository bzw.
image.tag in values.yaml bleiben als bewusster Einzelfall-Override moeglich (z.B. ein Test-Build
aus einer privaten Registry), sind aber normalerweise leer.
*/}}
{{- define "reservation-worker.image" -}}
{{- $repo := .Values.image.repository | default (printf "%s/ticketflow-reservation-worker" (required "ticketflow.imageRegistry fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageRegistry)) -}}
{{- $tag := .Values.image.tag | default (required "ticketflow.imageTag fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.imageTag) -}}
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
{{/*
Bootstrap-Adresse des Kafka-Clusters. Kafka laeuft in einem eigenen Namespace, getrennt von der
Anwendung - Name und Namespace kommen deshalb aus gitops/env/cluster.yaml statt als
ausgeschriebener DNS-Name hier in values.yaml zu stehen (er stand frueher in vier Charts parallel).
Port 9092 ist Strimzis "plain"-Listener (SASL_PLAINTEXT seit B2). config.kafkaBrokers ueberschreibt die Ableitung, falls noetig.
*/}}
{{- define "reservation-worker.kafkaBootstrap" -}}
{{- .Values.config.kafkaBrokers | default (printf "%s-kafka-bootstrap.%s.svc:9092" (required "ticketflow.kafka.clusterName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}

{{/*
URL der Apicurio-Registry - laeuft im selben Kafka-Namespace.
*/}}
{{- define "reservation-worker.schemaRegistryUrl" -}}
{{- .Values.config.registryUrl | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
{{/*
Postgres-Anbindung. Die Datenbank laeuft seit 2026-09-17 im SELBEN Namespace wie diese Anwendung
(gitops/env/cluster.yaml -> ticketflow.namespaces.app), weshalb hier weder ein eigener
Connection-String noch ein dupliziertes Credentials-Secret noetig ist: beide Werte leiten sich aus
ticketflow.postgres.databaseName ab und zeigen direkt auf das, was charts/postgres-cluster anlegt.
*/}}
{{- define "reservation-worker.databaseHost" -}}
{{- .Values.database.host | default (printf "%s-postgres-cluster-rw" (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}

{{- define "reservation-worker.databaseName" -}}
{{- .Values.database.name | default (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName) -}}
{{- end -}}

{{/*
Von charts/postgres-cluster erzeugtes Secret mit den Keys "username"/"password" - dasselbe, das
die CNPG-Cluster-CR als bootstrap-Secret nutzt. Kein chart-eigenes Duplikat mehr.
*/}}
{{- define "reservation-worker.databaseSecretName" -}}
{{- .Values.database.credentialsSecretName | default (printf "%s-postgres-user" (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}
