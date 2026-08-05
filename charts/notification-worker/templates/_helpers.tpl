{{/*
Erster Chart im Repo mit _helpers.tpl (die anderen volleigenen Charts,
cert-manager-config/argocd-ingress, rendern nur 1-2 Ressourcen und hardcoden Namen direkt aus
.Values). notification-worker rendert 6 Ressourcen, die alle konsistente Namen/Selector-Labels
brauchen - Standard helm-create-Helpers statt das an sechs Stellen zu duplizieren.
*/}}

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
