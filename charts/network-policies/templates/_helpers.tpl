{{/* Names from gitops/env/cluster.yaml; required, since wrong selectors silently drop traffic. */}}
{{- define "np.ns.app" -}}
{{- required "ticketflow.namespaces.app is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.app -}}
{{- end -}}
{{- define "np.ns.kafka" -}}
{{- required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka -}}
{{- end -}}
{{- define "np.ns.openbao" -}}
{{- required "ticketflow.namespaces.openbao is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.openbao -}}
{{- end -}}
{{- define "np.ns.externalSecrets" -}}
{{- required "ticketflow.namespaces.externalSecrets is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.externalSecrets -}}
{{- end -}}
{{- define "np.kafkaCluster" -}}
{{- required "ticketflow.kafka.clusterName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName -}}
{{- end -}}
{{/* CNPG cluster names, derived like in the postgres charts. */}}
{{- define "np.pgCluster" -}}
{{- printf "%s-postgres-cluster" (required "ticketflow.postgres.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName) -}}
{{- end -}}
{{- define "np.openbaoPgCluster" -}}
{{- printf "%s-postgres-cluster" (required "ticketflow.openbao.databaseName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.openbao.databaseName) -}}
{{- end -}}

{{- define "np.labels" -}}
app.kubernetes.io/name: network-policies
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/* Policy header. Usage: include "np.header" (list $ "<name>" "<namespace>") */}}
{{- define "np.header" -}}
{{- $root := index . 0 -}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ index . 1 }}
  namespace: {{ index . 2 }}
  labels:
    {{- include "np.labels" $root | nindent 4 }}
{{- end -}}

{{/*
Per namespace: default-deny (the only empty podSelector, intended) plus DNS for all pods. Usage:
include "np.baseline" (list $ "<namespace>")
*/}}
{{- define "np.baseline" -}}
{{- $root := index . 0 -}}
{{- $ns := index . 1 -}}
{{ include "np.header" (list $root "default-deny-all" $ns) }}
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
---
{{ include "np.header" (list $root "allow-dns-egress" $ns) }}
spec:
  podSelector: {}
  policyTypes:
    - Egress
  egress:
    - to:
        - {{ include "np.peer.coredns" $root | nindent 10 | trim }}
      ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
{{- end -}}

{{/* ----- Peers (from:/to: entries) ----- */}}

{{- define "np.nsSelector" -}}
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: {{ . }}
{{- end -}}

{{- define "np.peer.coredns" -}}
{{- include "np.nsSelector" .Values.namespaces.kubeSystem }}
podSelector:
  matchLabels:
    k8s-app: kube-dns
{{- end -}}

{{/* API server as source (webhooks, APIServices, proxy), see ticketflow.network. */}}
{{- define "np.peer.apiServer" -}}
{{- range (required "ticketflow.network.nodeCIDRs is missing - pass gitops/env/cluster.yaml" .Values.ticketflow.network.nodeCIDRs) }}
- ipBlock:
    cidr: {{ . }}
{{- end }}
{{- range (required "ticketflow.network.controlPlaneTunnelIPs is missing - pass gitops/env/cluster.yaml" .Values.ticketflow.network.controlPlaneTunnelIPs) }}
- ipBlock:
    cidr: {{ . }}
{{- end }}
{{- end -}}

{{- define "np.peer.nodes" -}}
{{- range (required "ticketflow.network.nodeCIDRs is missing - pass gitops/env/cluster.yaml" .Values.ticketflow.network.nodeCIDRs) }}
- ipBlock:
    cidr: {{ . }}
{{- end }}
{{- end -}}

{{/* Targets outside the cluster - only use with a port restriction. */}}
{{- define "np.peer.internet" -}}
- ipBlock:
    cidr: 0.0.0.0/0
- ipBlock:
    cidr: ::/0
{{- end -}}

{{- define "np.peer.prometheus" -}}
- {{ include "np.nsSelector" .Values.namespaces.monitoring | nindent 2 | trim }}
  podSelector:
    matchLabels:
      app.kubernetes.io/name: prometheus
{{- end -}}

{{- define "np.peer.traefik" -}}
- {{ include "np.nsSelector" .Values.namespaces.kubeSystem | nindent 2 | trim }}
  podSelector:
    matchLabels:
      app.kubernetes.io/name: traefik
{{- end -}}

{{/* All Kafka pods (brokers + controllers). */}}
{{- define "np.peer.kafkaNodes" -}}
- {{ include "np.nsSelector" (include "np.ns.kafka" .) | nindent 2 | trim }}
  podSelector:
    matchLabels:
      strimzi.io/cluster: {{ include "np.kafkaCluster" . }}
      strimzi.io/kind: Kafka
      strimzi.io/name: {{ include "np.kafkaCluster" . }}-kafka
{{- end -}}

{{/* Brokers only - clients talk to each broker directly after bootstrap. */}}
{{- define "np.peer.kafkaBrokers" -}}
- {{ include "np.nsSelector" (include "np.ns.kafka" .) | nindent 2 | trim }}
  podSelector:
    matchLabels:
      strimzi.io/cluster: {{ include "np.kafkaCluster" . }}
      strimzi.io/name: {{ include "np.kafkaCluster" . }}-kafka
      strimzi.io/broker-role: "true"
{{- end -}}

{{- define "np.peer.apicurio" -}}
- {{ include "np.nsSelector" (include "np.ns.kafka" .) | nindent 2 | trim }}
  podSelector:
    matchLabels:
      app.kubernetes.io/name: apicurio
{{- end -}}

{{- define "np.peer.cnpgOperator" -}}
- {{ include "np.nsSelector" .Values.namespaces.cnpg | nindent 2 | trim }}
  podSelector:
    matchLabels:
      app.kubernetes.io/name: cloudnative-pg
{{- end -}}

{{/* ----- Complete rules ----- */}}

{{/* Egress to the API server. */}}
{{- define "np.egress.apiServer" -}}
- to:
    {{- include "np.peer.nodes" . | nindent 4 }}
  ports:
    - port: {{ .Values.apiServerPort }}
      protocol: TCP
{{- end -}}

{{/*
Prometheus scrape ingress; renders nothing without namespaces.monitoring. Usage: include
"np.ingress.prometheus" (list $ 8080 9090)
*/}}
{{- define "np.ingress.prometheus" -}}
{{- $root := first . -}}
{{- if $root.Values.namespaces.monitoring }}
- from:
    {{- include "np.peer.prometheus" $root | nindent 4 }}
  ports:
    {{- range rest . }}
    - port: {{ . }}
      protocol: TCP
    {{- end }}
{{- end }}
{{- end -}}

{{/*
Ingress from the API server (webhook, APIService). Usage: include "np.ingress.apiServer" (list $
10250)
*/}}
{{- define "np.ingress.apiServer" -}}
{{- $root := first . -}}
- from:
    {{- include "np.peer.apiServer" $root | nindent 4 }}
  ports:
    {{- range rest . }}
    - port: {{ . }}
      protocol: TCP
    {{- end }}
{{- end -}}

{{/*
CNPG cluster per CNPG docs: operator -> 8000/5432, pods among themselves, instance manager -> API.
Usage: include "np.cnpgCluster" (dict "root" $ "ns" "<ns>" "cluster" "<name>" "clients" "<peers>")
*/}}
{{- define "np.cnpgCluster" -}}
{{- $root := .root -}}
{{ include "np.header" (list $root (printf "allow-%s" .cluster) .ns) }}
spec:
  podSelector:
    matchLabels:
      cnpg.io/cluster: {{ .cluster }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        {{- include "np.peer.cnpgOperator" $root | nindent 8 }}
      ports:
        - port: 8000
          protocol: TCP
        - port: 5432
          protocol: TCP
    - from:
        - podSelector:
            matchLabels:
              cnpg.io/cluster: {{ .cluster }}
    - from:
        {{- .clients | nindent 8 }}
      ports:
        - port: 5432
          protocol: TCP
    # `kubectl cnpg status` reads the instance manager via the API server proxy.
    {{- include "np.ingress.apiServer" (list $root 8000) | nindent 4 }}
    {{- include "np.ingress.prometheus" (list $root 9187) | nindent 4 }}
  egress:
    {{- include "np.egress.apiServer" $root | nindent 4 }}
    - to:
        - podSelector:
            matchLabels:
              cnpg.io/cluster: {{ .cluster }}
{{- end -}}
