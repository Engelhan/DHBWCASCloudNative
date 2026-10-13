# Chart: kafka-cluster

Eigenständiger Chart für Kafka (Messaging) sowie die Registrierung der kanonischen Kafka-Event-
Schemas in Apicurio Registry (Carlos' Scope). `catalog-api`/`frontend` haben einen eigenen Chart
(`charts/ticketflow-app/`) — Team-Entscheidung, nicht in derselben Application wie Kafka
deployen. `notification-worker`, `reservation-worker`
und der CNPG-Postgres-Cluster sind ebenfalls eigene, eigenständige Charts
(`charts/notification-worker/`, `charts/reservation-worker/`, `charts/postgres-cluster/`) — siehe
`../README.md`.

## Struktur

```
kafka-cluster/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # Lab-Profil: 3 Broker/3 Controller, kleiner Storage
├── values-prod.yaml         # 3 Broker/3 Controller, größerer Storage
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen
├── files/schemas/            # kanonische JSON-Schemas (reservation-requested, reservation-result)
└── templates/
    ├── messaging/                         # Strimzi Kafka (KRaft, kein ZooKeeper)
    │   ├── kafka-broker-nodepool.yaml
    │   ├── kafka-controller-nodepool.yaml
    │   ├── kafka-cluster.yaml              # SCRAM-SHA-512-Auth + authorization: simple (B4)
    │   ├── kafka-metrics-configmap.yaml   # JMX-Exporter-Regeln (derzeit scrapt sie kein Prometheus)
    │   ├── kafka-topics.yaml              # aus .Values.kafka.topics generiert
    │   ├── kafka-users.yaml               # aus .Values.kafka.users generiert (B4, KafkaUser+ACLs)
    │   └── kafka-user-pushsecrets.yaml    # spiegelt die Strimzi-SCRAM-Secrets nach OpenBao (B3)
    └── schema-registry/
        ├── configmap.yaml
        └── register-job.yaml               # registriert files/schemas/* in Apicurio (Helm-Hook)
```

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `kafka.controller/broker.replicas` | 3 | 3 | beide: ein Pod je simulierter Region (P4, harte `topologySpreadConstraint`); 3 Controller = kleinstes KRaft-Quorum, das einen Ausfall übersteht |
| Topic-`replicationFactor` / `min.insync.replicas` | 3 / 2 | 3 / 2 | ein Broker darf ausfallen oder per Rolling Update neu starten, ohne dass Produce mit `acks=all` scheitert (P5) |
| `partitions` | 20 | 20 | Obergrenze paralleler Consumer je Group; KEDA `maxReplicaCount` in `charts/reservation-worker` ist 10, jeder Pod bekommt genau 2 Partitionen. Kommt in beiden Profilen aus `values.yaml` — Kafka kann Partitionen nie reduzieren, ein kleinerer prod-Wert ließe sich auf einem bestehenden Cluster nicht anwenden |
| Broker-/Controller-Storage | 5Gi / 2Gi | 10Gi / 5Gi | dev: Lab-Longhorn-Platz; Herleitung siehe `values.yaml` |

Requests/Limits/Heap stehen mit Herleitung in `values.yaml` (`kafka.broker`/`kafka.controller`).

## Verteilung und Replikation prüfen

P4: ein Broker/Controller je Region; P5: alle Partitionen mit 3 In-Sync-Replicas:

```bash
export KUBECONFIG=infra/ansible/kubeconfig
kubectl -n kafka get pods -l strimzi.io/cluster=ticketflow-kafka -o wide
kubectl get nodes -L topology.ticketflow/region
# Pod-Namen enthalten die Strimzi-Node-ID, die je nach Anlage-Reihenfolge nicht bei 0 beginnt
B=$(kubectl -n kafka get pod -l strimzi.io/pool-name=broker -o name | head -1)
kubectl -n kafka exec "$B" -c kafka -- bin/kafka-topics.sh --bootstrap-server localhost:9093 --describe --topic reservation-requested
```

## Verwendung

```bash
helm lint charts/kafka-cluster \
  -f charts/kafka-cluster/values.yaml \
  -f charts/kafka-cluster/values-dev.yaml \
  -f gitops/env/cluster.yaml
```

Deployment läuft indirekt über ArgoCD (Element `kafka-cluster` in `gitops/applicationsets/`),
Namespace `kafka`.

## Namespaces

Der Chart rendert bewusst in **zwei** Namespaces:

| Ressourcen | Namespace | warum |
|---|---|---|
| `Kafka`, `KafkaNodePool`, `KafkaTopic`, Metrics-ConfigMap, Schema-Registrar-Job | `ticketflow.namespaces.kafka` (= Release-Namespace) | die Kafka-Plattform läuft getrennt von der Anwendung |
| `KafkaUser` + deren `PushSecret`s nach OpenBao | `ticketflow.namespaces.app` | siehe unten |

Der Strimzi User Operator legt das generierte SCRAM-`Secret` **immer im Namespace der
`KafkaUser`-CR** ab, und Kubernetes erlaubt kein `secretKeyRef` über Namespace-Grenzen. Lägen die
`KafkaUser`s beim Cluster, kämen `catalog-api`/`reservation-worker`/`notification-worker` nicht
mehr an ihr Passwort — dieselbe Grenze, die schon die Postgres-Credentials betrifft. Damit der
User Operator die CRs im anderen Namespace überhaupt sieht, setzt `kafka-cluster.yaml`
`spec.entityOperator.userOperator.watchedNamespace` auf den App-Namespace. **Ohne dieses Feld
würden die `KafkaUser`s nie reconciled — ohne Fehlermeldung, nur mit dauerhaft leerem `status`
und fehlenden Secrets.**

Das Feld allein genügt aber nicht: der Cluster-Operator behandelt namespace-übergreifendes
Beobachten als abgeschaltetes Feature und **löscht** das Entity-Operator-Deployment wieder, sobald
Topic- oder User-Operator ein fremdes `watchedNamespace` tragen. Im Log steht dann

```
Entity Operator deployment deleted because Topic Operator and/or User Operator are configured
with watchedNamespace set to a different namespace but the feature is disabled. To enable
cross-namespace watching, set STRIMZI_ENTITY_OPERATOR_WATCHED_NAMESPACE_ENABLED=true
```

Diese Env-Variable setzt `charts/strimzi-operator/values.yaml` über `extraEnvs` am
Cluster-Operator-Deployment — der Upstream-Chart hat dafür keinen eigenen Wert. Die RBAC passt
bereits ohne Zutun: bei `watchAnyNamespace: true` erzeugt der Chart für die
Entity-Operator-Delegation ein `ClusterRoleBinding` statt eines namespace-gebundenen
`RoleBinding`s. **Beide Stellen gehören zusammen** — wer `watchedNamespace` ändert oder entfernt,
muss die Env-Variable mitdenken, und umgekehrt.

Namespace und Cluster-Name kommen aus `gitops/env/cluster.yaml`
(`ticketflow.namespaces.{app,kafka}`, `ticketflow.kafka.clusterName`); alle Clients leiten ihre
Bootstrap-Adresse daraus ab, statt sie auszuschreiben.
