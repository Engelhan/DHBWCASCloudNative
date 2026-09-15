# Chart: kafka-cluster

Volleigener Chart für die Strimzi-Kafka-Cluster-Instanz (Kafka-CR, KafkaNodePools, KafkaTopics)
sowie die Apicurio-Schema-Registrierung. Getrennt vom Strimzi-Operator (`charts/strimzi-operator/`)
und vom App-Layer (`charts/ticketflow/`) — analog zu `charts/postgres-cluster/`, das genauso vom
CNPG-Operator (`charts/cnpg-operator/`) getrennt ist. Ausgelagert aus `charts/ticketflow` (2026-09-15,
siehe dessen Git-Historie), das ursprünglich Kafka + `catalog-api`/`frontend` in einem einzigen
Helm-Release bündelte.

## Struktur

```
kafka-cluster/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # 1 Kafka-Broker/-Controller
├── values-prod.yaml         # HA: 3 Kafka-Broker/-Controller
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen
├── files/schemas/            # kanonische JSON-Schemas fuer die Kafka-Event-Contracts
│   ├── reservation-requested.schema.json
│   └── reservation-result.schema.json
└── templates/
    ├── kafka-broker-nodepool.yaml
    ├── kafka-controller-nodepool.yaml
    ├── kafka-cluster.yaml                  # Kafka-CR (KRaft, kein ZooKeeper)
    ├── kafka-metrics-configmap.yaml        # JMX-Prometheus-Exporter-Regeln (P7)
    ├── kafka-topics.yaml                   # aus .Values.kafka.topics generiert
    └── schema-registry/
        ├── configmap.yaml                  # traegt files/schemas/ in den Cluster
        └── register-job.yaml               # registriert sie idempotent in Apicurio (Helm-Hook)
```

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `kafka.controller/broker.replicas` | 1 | 3 | dev: ein Broker reicht für Funktionstests; prod: Ausfallsicherheit (P5) |
| `kafka.topics[].partitions` | 3 | 8 | prod: genug Partitionen, damit KEDA `reservation-worker` bis auf 8 Pods parallelisieren kann |
| `kafka.config.*ReplicationFactor`/`minInsyncReplicas` | 1 | 3/2 | prod: toleriert den Ausfall eines Brokers ohne Produce-Fehlschlag bei `acks=all` (P5) |

## Verwendung

```bash
helm lint charts/kafka-cluster -f charts/kafka-cluster/values-dev.yaml
helm template kafka-cluster charts/kafka-cluster -f charts/kafka-cluster/values-dev.yaml
```

Deployment läuft indirekt über ArgoCD (`gitops/apps/kafka-cluster.yaml`, `targetRevision: main`,
Namespace `ticketflow`, `valueFiles: [values-dev.yaml]`).

Andere Services referenzieren den Kafka-Bootstrap-Service über den festen String
`ticketflow-kafka-kafka-bootstrap.ticketflow.svc:9092` (kein Helm-Cross-Chart-Lookup) — solange
`kafka.metadata.name`/`kafka.metadata.namespace` hier unverändert bleiben, ändert die Auslagerung
aus `charts/ticketflow` daran nichts.
