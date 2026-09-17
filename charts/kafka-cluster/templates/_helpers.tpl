{{/*
Zentrale Ableitungen aus gitops/env/cluster.yaml. Namespace und Cluster-Name standen frueher
doppelt in values.yaml (als `namespace` UND `kafka.metadata.namespace`) und zusaetzlich als
ausgeschriebener DNS-Name in vier weiteren Charts - beim Trennen der Namespaces waeren das fuenf
Stellen gewesen, die gleichzeitig haetten stimmen muessen.
*/}}

{{/* Namespace der Kafka-Plattform - hier laeuft alles ausser den KafkaUser-CRs. */}}
{{- define "kafka-cluster.namespace" -}}
{{- required "ticketflow.namespaces.kafka fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.kafka -}}
{{- end -}}

{{/*
Namespace der Anwendung. Die KafkaUser-CRs werden bewusst DORT angelegt, nicht beim Cluster: der
Strimzi User Operator legt das generierte SCRAM-Secret immer im Namespace der KafkaUser-CR ab, und
Kubernetes erlaubt kein secretKeyRef ueber Namespace-Grenzen. Laegen die User beim Cluster, kaemen
catalog-api/reservation-worker/notification-worker nicht mehr an ihr Passwort - dieselbe Grenze,
die schon die Postgres-Credentials betrifft. Damit der User Operator sie dort ueberhaupt sieht,
setzt kafka-cluster.yaml entsprechend spec.entityOperator.userOperator.watchedNamespace.
*/}}
{{- define "kafka-cluster.appNamespace" -}}
{{- required "ticketflow.namespaces.app fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.namespaces.app -}}
{{- end -}}

{{/* Name der Kafka-CR. */}}
{{- define "kafka-cluster.clusterName" -}}
{{- required "ticketflow.kafka.clusterName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.kafka.clusterName -}}
{{- end -}}

{{/* URL der Apicurio-Registry (charts/apicurio laeuft im Kafka-Namespace). */}}
{{- define "kafka-cluster.schemaRegistryUrl" -}}
{{- .Values.schemaRegistry.url | default (printf "http://apicurio.%s.svc:8080/apis/registry/v3" (include "kafka-cluster.namespace" .)) -}}
{{- end -}}
