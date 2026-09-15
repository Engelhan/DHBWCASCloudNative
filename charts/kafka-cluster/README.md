# Chart: kafka-cluster

Eigenständiger Chart für Kafka (Messaging) sowie die Registrierung der kanonischen Kafka-Event-
Schemas in Apicurio Registry (Carlos' Scope). `catalog-api`/`frontend` haben einen eigenen Chart
(`charts/ticketflow-app/`) — Team-Entscheidung, nicht in derselben Application wie Kafka
deployen, siehe `gitops/apps/ticketflow-app.yaml`. `notification-worker`, `reservation-worker`
und der CNPG-Postgres-Cluster sind ebenfalls eigene, eigenständige Charts
(`charts/notification-worker/`, `charts/reservation-worker/`, `charts/postgres-cluster/`) — siehe
`../README.md`.

## Struktur

```
kafka-cluster/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # kleine Replica-/Ressourcenwerte, 1 Kafka-Broker
├── values-prod.yaml         # HA: 3 Kafka-Broker
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen
├── files/schemas/            # kanonische JSON-Schemas (reservation-requested, reservation-result)
└── templates/
    ├── messaging/                         # Strimzi Kafka (KRaft, kein ZooKeeper)
    │   ├── kafka-broker-nodepool.yaml
    │   ├── kafka-controller-nodepool.yaml
    │   ├── kafka-cluster.yaml
    │   ├── kafka-metrics-configmap.yaml   # JMX-Prometheus-Exporter-Regeln (P7)
    │   └── kafka-topics.yaml              # aus .Values.kafka.topics generiert
    └── schema-registry/
        ├── configmap.yaml
        └── register-job.yaml               # registriert files/schemas/* in Apicurio (Helm-Hook)
```

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `kafka.controller/broker.replicas` | 1 | 3 | dev: ein Broker reicht für Funktionstests; prod: Ausfallsicherheit (P5) |

## Verwendung

```bash
helm lint charts/kafka-cluster -f charts/kafka-cluster/values-dev.yaml
helm template kafka-cluster charts/kafka-cluster -f charts/kafka-cluster/values-dev.yaml
```

Deployment läuft indirekt über ArgoCD (`gitops/apps/kafka-cluster.yaml`, `targetRevision: main`,
Namespace `ticketflow`).
