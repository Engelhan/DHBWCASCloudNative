{{/* Derived from gitops/env/cluster.yaml. */}}

{{/* Kafka platform namespace (everything except the KafkaUsers). */}}
{{- define "kafka-cluster.namespace" -}}
{{- required "ticketflow.namespaces.kafka is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka -}}
{{- end -}}

{{/*
App namespace. KafkaUsers live here because Strimzi writes the SCRAM secret next to the CR and the
services read it via secretKeyRef (needs userOperator.watchedNamespace).
*/}}
{{- define "kafka-cluster.appNamespace" -}}
{{- required "ticketflow.namespaces.app is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.app -}}
{{- end -}}

{{- define "kafka-cluster.clusterName" -}}
{{- required "ticketflow.kafka.clusterName is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName -}}
{{- end -}}

{{/* Apicurio registry URL (same namespace as Kafka). */}}
{{- define "kafka-cluster.schemaRegistryUrl" -}}
{{- .Values.schemaRegistry.url | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (include "kafka-cluster.namespace" .)) -}}
{{- end -}}
