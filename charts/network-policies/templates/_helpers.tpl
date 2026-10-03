{{/*
Namespaces und Namen aus gitops/env/cluster.yaml - dieselben Werte, aus denen die jeweiligen
Workload-Charts ihre Ressourcen ableiten. Stimmen sie hier nicht, greifen die Selektoren ins Leere
und der Traffic wird verworfen, deshalb required statt Default.
*/}}
{{- define "np.ns.app" -}}
{{- required "ticketflow.namespaces.app fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.app -}}
{{- end -}}
{{- define "np.ns.kafka" -}}
{{- required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka -}}
{{- end -}}
{{- define "np.ns.openbao" -}}
{{- required "ticketflow.namespaces.openbao fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.openbao -}}
{{- end -}}
{{- define "np.ns.externalSecrets" -}}
{{- required "ticketflow.namespaces.externalSecrets fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.externalSecrets -}}
{{- end -}}
{{- define "np.kafkaCluster" -}}
{{- required "ticketflow.kafka.clusterName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName -}}
{{- end -}}
{{/* CNPG-Cluster-CR-Namen, gleiche Ableitung wie charts/postgres-cluster bzw. charts/openbao-postgres. */}}
{{- define "np.pgCluster" -}}
{{- printf "%s-postgres-cluster" (required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName) -}}
{{- end -}}
{{- define "np.openbaoPgCluster" -}}
{{- printf "%s-postgres-cluster" (required "ticketflow.openbao.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.openbao.databaseName) -}}
{{- end -}}

{{- define "np.labels" -}}
app.kubernetes.io/name: network-policies
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/*
Kopf einer NetworkPolicy. Aufruf: include "np.header" (list $ "<name>" "<namespace>")
*/}}
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
Grundausstattung jedes Namespaces: alles verbieten (einzige Stelle mit leerem podSelector - das
ist hier gewollt, anders als beim dokumentierten Loki-Vorfall) plus DNS zu CoreDNS fuer alle Pods,
weil praktisch jede Verbindung mit einer Namensaufloesung beginnt.
Aufruf: include "np.baseline" (list $ "<namespace>")
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

{{/* ----- Peers (Eintraege fuer from:/to:) ----- */}}

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

{{/*
Der API-Server als Quelle (Admission-Webhooks, APIServices, API-Proxy). Siehe
gitops/env/cluster.yaml ticketflow.network fuer die Begruendung beider Adressbereiche.
*/}}
{{- define "np.peer.apiServer" -}}
{{- range (required "ticketflow.network.nodeCIDRs fehlt - gitops/env/cluster.yaml mitgeben" .Values.ticketflow.network.nodeCIDRs) }}
- ipBlock:
    cidr: {{ . }}
{{- end }}
{{- range (required "ticketflow.network.controlPlaneTunnelIPs fehlt - gitops/env/cluster.yaml mitgeben" .Values.ticketflow.network.controlPlaneTunnelIPs) }}
- ipBlock:
    cidr: {{ . }}
{{- end }}
{{- end -}}

{{- define "np.peer.nodes" -}}
{{- range (required "ticketflow.network.nodeCIDRs fehlt - gitops/env/cluster.yaml mitgeben" .Values.ticketflow.network.nodeCIDRs) }}
- ipBlock:
    cidr: {{ . }}
{{- end }}
{{- end -}}

{{/* Ziele ausserhalb des Clusters (ACME, DNS, GitHub, Helm-Repos) - nur mit Portbeschraenkung verwenden. */}}
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

{{/* Alle Kafka-Pods (Broker + Controller) des Strimzi-Clusters. */}}
{{- define "np.peer.kafkaNodes" -}}
- {{ include "np.nsSelector" (include "np.ns.kafka" .) | nindent 2 | trim }}
  podSelector:
    matchLabels:
      strimzi.io/cluster: {{ include "np.kafkaCluster" . }}
      strimzi.io/kind: Kafka
      strimzi.io/name: {{ include "np.kafkaCluster" . }}-kafka
{{- end -}}

{{/* Nur die Broker - Clients sprechen nach dem Bootstrap jeden Broker direkt an. */}}
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

{{/* ----- Komplette Regeln ----- */}}

{{/* Egress zum API-Server. Jeder Operator/Controller braucht das. */}}
{{- define "np.egress.apiServer" -}}
- to:
    {{- include "np.peer.nodes" . | nindent 4 }}
  ports:
    - port: {{ .Values.apiServerPort }}
      protocol: TCP
{{- end -}}

{{/*
Ingress fuer Prometheus-Scrapes auf den angegebenen Ports.
Aufruf: include "np.ingress.prometheus" (list $ 8080 9090)
Rendert nichts, solange namespaces.monitoring nicht gesetzt ist (kein Prometheus im Cluster).
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
Ingress vom API-Server auf den angegebenen Ports (Webhook, APIService).
Aufruf: include "np.ingress.apiServer" (list $ 10250)
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
Ein CNPG-Postgres-Cluster (App-DB oder OpenBao-DB), gemaess CNPG-Doku (docs/src/networking.md,
samples/networkpolicy-example.yaml): der Operator muss jeden Instanz-Pod auf 8000 und 5432
erreichen, und die Pods muessen untereinander sprechen koennen (Streaming-Replikation, Join-/
initdb-Jobs, die CNPG mit demselben cnpg.io/cluster-Label startet). Der Instance-Manager in jedem
Pod ist selbst ein Kubernetes-Controller und braucht deshalb den API-Server.
Aufruf: include "np.cnpgCluster" (dict "root" $ "ns" "<ns>" "cluster" "<name>" "clients" "<peer-yaml>")
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
    # `kubectl cnpg status` liest den Instance-Manager ueber den API-Server-Proxy.
    {{- include "np.ingress.apiServer" (list $root 8000) | nindent 4 }}
    {{- include "np.ingress.prometheus" (list $root 9187) | nindent 4 }}
  egress:
    {{- include "np.egress.apiServer" $root | nindent 4 }}
    - to:
        - podSelector:
            matchLabels:
              cnpg.io/cluster: {{ .cluster }}
{{- end -}}
