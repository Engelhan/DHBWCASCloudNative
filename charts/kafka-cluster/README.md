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
    │   ├── kafka-cluster.yaml              # SCRAM-SHA-512-Auth + authorization: simple (B2)
    │   ├── kafka-metrics-configmap.yaml   # JMX-Prometheus-Exporter-Regeln (P7)
    │   ├── kafka-topics.yaml              # aus .Values.kafka.topics generiert
    │   ├── kafka-users.yaml               # aus .Values.kafka.users generiert (B2, KafkaUser+ACLs)
    │   └── kafka-akhq-user-secret.yaml    # vorgegebenes SCRAM-Passwort fuer die akhq-KafkaUser
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
helm lint charts/kafka-cluster \
  -f charts/kafka-cluster/values.yaml \
  -f charts/kafka-cluster/values-dev.yaml \
  -f gitops/env/cluster.yaml
```

Deployment läuft indirekt über ArgoCD (Element `kafka-cluster` in `gitops/applicationset.yaml`),
Namespace `kafka`.

## Namespaces

Der Chart rendert bewusst in **zwei** Namespaces:

| Ressourcen | Namespace | warum |
|---|---|---|
| `Kafka`, `KafkaNodePool`, `KafkaTopic`, Metrics-ConfigMap, Schema-Registrar-Job | `ticketflow.namespaces.kafka` (= Release-Namespace) | die Kafka-Plattform läuft getrennt von der Anwendung |
| `KafkaUser` + das AKHQ-Passwort-`Secret` | `ticketflow.namespaces.app` | siehe unten |

Der Strimzi User Operator legt das generierte SCRAM-`Secret` **immer im Namespace der
`KafkaUser`-CR** ab, und Kubernetes erlaubt kein `secretKeyRef` über Namespace-Grenzen. Lägen die
`KafkaUser`s beim Cluster, kämen `catalog-api`/`reservation-worker`/`notification-worker` nicht
mehr an ihr Passwort — dieselbe Grenze, die schon die Postgres-Credentials betrifft. Damit der
User Operator die CRs im anderen Namespace überhaupt sieht, setzt `kafka-cluster.yaml`
`spec.entityOperator.userOperator.watchedNamespace` auf den App-Namespace. **Ohne dieses Feld
würden die `KafkaUser`s nie reconciled — ohne Fehlermeldung, nur mit dauerhaft leerem `status`
und fehlenden Secrets.**

Namespace und Cluster-Name kommen aus `gitops/env/cluster.yaml`
(`ticketflow.namespaces.{app,kafka}`, `ticketflow.kafka.clusterName`); alle Clients leiten ihre
Bootstrap-Adresse daraus ab, statt sie auszuschreiben.
