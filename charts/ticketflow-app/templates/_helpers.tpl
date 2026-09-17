{{/*
Basis-Helper, identisch zu charts/reservation-worker und charts/notification-worker - dieser Chart
rendert allerdings ZWEI Komponenten (catalog-api, frontend) in einem Release, weshalb es unter den
Basis-Helpern zusaetzlich komponentenbezogene Varianten gibt. Diese bekommen ihren Kontext als
dict uebergeben:

    {{ include "ticketflow-app.componentLabels" (dict "root" . "component" "catalog-api") }}

Vorher standen Name und Labels in allen elf Templates woertlich drin.
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

{{/* --- Komponentenbezogene Varianten (dict mit "root" und "component") --- */}}

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

{{/*
Vollstaendiger Image-Name einer Komponente. Registry und Tag kommen zentral aus
gitops/env/cluster.yaml (ticketflow.imageRegistry / .imageTag), damit ein Image-Bump nur EINE
Stelle im Repo beruehrt - .github/workflows/build-images.yml schreibt den Tag dort automatisch
fort. image.repository/.tag der Komponente bleiben als bewusster Einzelfall-Override moeglich,
sind aber normalerweise leer.
*/}}
{{- define "ticketflow-app.componentImage" -}}
{{- $repo := .image.repository | default (printf "%s/ticketflow-%s" (required "ticketflow.imageRegistry fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .root.Values.ticketflow.imageRegistry) .component) -}}
{{- $tag := .image.tag | default (required "ticketflow.imageTag fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .root.Values.ticketflow.imageTag) -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{/*
Oeffentlicher Hostname der Anwendung. Leitet sich per Default aus der zentralen DNS-Zone ab, damit
ein Fork ihn nicht separat pflegen muss; frontend.ingress.host ueberschreibt ihn bei Bedarf.
*/}}
{{- define "ticketflow-app.ingressHost" -}}
{{- .Values.frontend.ingress.host | default (printf "ticketflow.%s" (required "ticketflow.dnsZone fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.dnsZone)) -}}
{{- end -}}

{{/*
Bootstrap-Adresse des Kafka-Clusters. Kafka laeuft in einem eigenen Namespace, getrennt von dieser
Anwendung - Name und Namespace kommen deshalb aus gitops/env/cluster.yaml statt als
ausgeschriebener DNS-Name in values.yaml zu stehen (er stand frueher in vier Charts parallel).
Port 9092 ist Strimzis "plain"-Listener (seit B2 SASL_PLAINTEXT).
*/}}
{{- define "ticketflow-app.kafkaBootstrap" -}}
{{- .Values.catalogApi.config.kafkaBrokers | default (printf "%s-kafka-bootstrap.%s.svc:9092" (required "ticketflow.kafka.clusterName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName) (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}

{{/* URL der Apicurio-Registry - laeuft im selben Kafka-Namespace. */}}
{{- define "ticketflow-app.schemaRegistryUrl" -}}
{{- .Values.catalogApi.config.registryUrl | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka)) -}}
{{- end -}}
{{/*
Postgres-Anbindung. Die Datenbank laeuft seit 2026-09-17 im SELBEN Namespace wie diese Anwendung
(gitops/env/cluster.yaml -> ticketflow.namespaces.app), weshalb hier weder ein eigener
Connection-String noch ein dupliziertes Credentials-Secret noetig ist: beide Werte leiten sich aus
ticketflow.postgres.databaseName ab und zeigen direkt auf das, was charts/postgres-cluster anlegt.
*/}}
{{- define "ticketflow-app.databaseHost" -}}
{{- .Values.catalogApi.database.host | default (printf "%s-postgres-cluster-rw" (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}

{{- define "ticketflow-app.databaseName" -}}
{{- .Values.catalogApi.database.name | default (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName) -}}
{{- end -}}

{{/*
Von charts/postgres-cluster erzeugtes Secret mit den Keys "username"/"password" - dasselbe, das
die CNPG-Cluster-CR als bootstrap-Secret nutzt. Kein chart-eigenes Duplikat mehr.
*/}}
{{- define "ticketflow-app.databaseSecretName" -}}
{{- .Values.catalogApi.database.credentialsSecretName | default (printf "%s-postgres-user" (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName)) -}}
{{- end -}}
