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
├── values-dev.yaml          # Lab-Profil: 3 Broker/3 Controller, kleiner Storage, 3 Partitionen
├── values-prod.yaml         # 3 Broker/3 Controller, groesserer Storage, 8 Partitionen
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
    │   └── kafka-user-pushsecrets.yaml    # spiegelt die Strimzi-SCRAM-Secrets nach OpenBao (B1)
    └── schema-registry/
        ├── configmap.yaml
        └── register-job.yaml               # registriert files/schemas/* in Apicurio (Helm-Hook)
```

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `kafka.controller/broker.replicas` | 3 | 3 | beide: ein Pod je simulierter Region (P4, harte `topologySpreadConstraint`); 3 Controller = kleinstes KRaft-Quorum, das einen Ausfall übersteht |
| Topic-`replicationFactor` / `min.insync.replicas` | 3 / 2 | 3 / 2 | ein Broker darf ausfallen oder per Rolling Update neu starten, ohne dass Produce mit `acks=all` scheitert (P5) |
| `partitions` | 3 | 8 | Obergrenze paralleler Consumer je Group = KEDA `maxReplicaCount` in `charts/reservation-worker` |
| Broker-/Controller-Storage | 5Gi / 2Gi | 10Gi / 5Gi | dev: Lab-Longhorn-Platz; Herleitung siehe `values.yaml` |

dev lief bis 2026-09-29 mit einem einzigen Broker/Controller. Das hat P4 (keine Verteilung sichtbar,
die harte Constraint war per `if replicas > 1` abgeschaltet) und P5 (jeder Broker-Neustart = Ausfall
des Reservierungs-Eingangs) auf genau dem Cluster verfehlt, auf dem beides gezeigt werden muss.
Requests/Limits/Heap stehen mit Herleitung in `values.yaml` (`kafka.broker`/`kafka.controller`).

## Umstellung von 1 auf 3 Broker (einmalig, bestehender Cluster)

Ein ArgoCD-Sync allein reicht **nicht** vollständig: neue Broker/Controller kommen dazu, aber

- Strimzis Topic Operator kann den `replicationFactor` eines **bestehenden** Topics nur mit Cruise
  Control ändern (hier nicht deployt) — `reservation-requested`/`reservation-result` blieben bei 1
  und die `KafkaTopic`s meldeten `NotReady`.
- `offsets.topic.replication.factor` wirkt nur beim ersten Anlegen von `__consumer_offsets` - das
  bliebe bei Replication-Factor 1, ein Broker-Verlust würde alle Consumer-Offsets mitnehmen.
- Ein KRaft-Controller-Quorum lässt sich nicht per `replicas: 1 -> 3` vergrößern wie ein Broker-Pool.

Deshalb Kafka einmal frisch aufsetzen (kein Lastverkehr währenddessen; Reservierungen selbst liegen in
Postgres und bleiben erhalten, verloren gehen nur noch unverarbeitete Events und die Apicurio-Schemas,
die der Register-Hook beim Sync neu anlegt):

```bash
export KUBECONFIG=infra/ansible/kubeconfig
# 1. Auto-Sync kurz aus. Die Application gehoert der ApplicationSet - ein Patch an ihr allein
#    wuerde vom ApplicationSet-Controller sofort zurueckgesetzt, deshalb ihn vorher anhalten.
kubectl -n argocd scale deploy argocd-applicationset-controller --replicas=0
kubectl -n argocd patch app kafka-cluster --type=merge -p '{"spec":{"syncPolicy":{"automated":null}}}'
# 2. Kafka-CR + NodePools loeschen (Strimzi raeumt die Pods ab), danach die PVCs (deleteClaim: false)
kubectl -n kafka delete kafka ticketflow-kafka
kubectl -n kafka delete kafkanodepool broker controller
kubectl -n kafka wait --for=delete pod -l strimzi.io/cluster=ticketflow-kafka --timeout=300s
kubectl -n kafka delete pvc -l strimzi.io/cluster=ticketflow-kafka
# 3. ApplicationSet-Controller wieder an - er stellt syncPolicy.automated selbst wieder her,
#    ArgoCD legt den Cluster mit 3+3 Pods und RF 3 neu an
kubectl -n argocd scale deploy argocd-applicationset-controller --replicas=1
kubectl -n kafka get pods -l strimzi.io/cluster=ticketflow-kafka -w
# 4. Apicurio liest seinen kafkasql-Journal-Topic nur beim Start - danach neu starten, dann die
#    Clients, damit sie die (neu vergebenen) Schema-globalIds auflösen
kubectl -n kafka rollout restart deploy -l app.kubernetes.io/name=apicurio
kubectl -n ticketflow rollout restart deploy   # catalog-api, frontend, beide Worker
```

Prüfen (P4: ein Broker/Controller je Region; P5: alle Partitionen mit 3 In-Sync-Replicas):

```bash
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
