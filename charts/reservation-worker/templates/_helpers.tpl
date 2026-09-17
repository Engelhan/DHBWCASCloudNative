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
