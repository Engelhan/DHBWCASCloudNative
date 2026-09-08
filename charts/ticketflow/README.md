# Chart: ticketflow

Umbrella-Helm-Chart für die gesamte TicketFlow-Anwendungsebene.

**Hinweis:** `notification-worker` hat inzwischen einen eigenen, eigenständigen Chart
(`charts/notification-worker/`) bekommen, statt Teil dieses Umbrella-Charts zu sein — siehe
`../README.md`. Die Struktur unten ist entsprechend nur noch für `catalog-api` und
`reservation-worker` (beide noch nicht implementiert) sowie die CNPG-/Kafka-CRs gültig.

## Geplante Struktur

```
ticketflow/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # kleine Replica-/Ressourcenwerte, 1 Kafka-Broker, 1 CNPG-Instanz
├── values-prod.yaml         # HA: 3 Kafka-Broker, CNPG Primary+2 Replicas, höhere Requests/Limits
├── values.schema.json       # JSON-Schema: validiert z.B. Replica-Zahlen, Pflichtfelder für Secrets
└── templates/
    ├── catalog-api/         # Deployment, Service, ConfigMap, ServiceMonitor
    ├── reservation-worker/  # Deployment, ScaledObject (KEDA), ServiceMonitor
    ├── frontend/             # Deployment, Service, Ingress
    ├── database/
    │   └── cnpg-cluster.yaml # CNPG `Cluster`-CR (PVC-Größe, Instanzanzahl aus values) — TODO
    ├── messaging/                       # fertig, siehe unten
    │   ├── kafka-broker-nodepool.yaml     # Strimzi `KafkaNodePool`, role broker
    │   ├── kafka-controller-nodepool.yaml # Strimzi `KafkaNodePool`, role controller
    │   ├── kafka-cluster.yaml             # Strimzi `Kafka`-CR, KRaft-Modus (kein ZooKeeper)
    │   ├── kafka-metrics-configmap.yaml   # JMX-Prometheus-Exporter-Regeln (P7)
    │   └── kafka-topics.yaml              # `KafkaTopic`-CRs, aus `.Values.kafka.topics` generiert
    ├── config/
    │   ├── configmap.yaml
    │   └── secret.yaml        # Platzhalter-Struktur; echte Werte kommen aus External Secret /
    │                          # `helm install --set-file` zur Deploy-Zeit, nicht aus dem Repo
    └── monitoring/
        ├── grafana-dashboard-configmap.yaml  # bindet observability/dashboards/*.json ein
        └── prometheusrule.yaml               # bindet observability/alerts/*.yaml ein
```

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `reservationWorker.replicas` (min/max für KEDA) | 1–2 | 2–8 | dev muss nur funktional zeigen, dass Scaling greift; prod zeigt die tatsächliche Kapazität |
| `kafka.replicas` | 1 | 3 | dev: ein Broker reicht für Funktionstests; prod: Ausfallsicherheit (P5) |
| `postgres.instances` | 1 | 3 (Primary+2) | gleiche Begründung wie Kafka |
| `resources.requests/limits` | knapp bemessen | großzügiger | dev läuft auf kleineren VM-Flavors |

`values.schema.json` erzwingt z.B., dass `kafka.replicas >= 1` und `reservationWorker.minReplicas
<= maxReplicas` gelten, damit ein fehlerhafter Values-Override nicht erst beim `kubectl apply`
auffällt.

## Verwendung (geplant, i.d.R. indirekt über ArgoCD)

```bash
helm lint charts/ticketflow -f charts/ticketflow/values-dev.yaml
helm template ticketflow charts/ticketflow -f charts/ticketflow/values-prod.yaml
```

`messaging/` ist fertig (Kafka-Cluster, siehe `gitops/apps/ticketflow.yaml`, aktuell auf Branch
`strimzi-setup`). `TODO`: restliche Templates (Services, `database/`, `config/`, `monitoring/`)
schreiben, sobald die drei Services ein `/metrics`- und `/healthz`-Endpoint haben und die
CNPG-Operator-CRD-Version feststeht.
