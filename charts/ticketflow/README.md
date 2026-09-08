# Chart: ticketflow

Umbrella-Helm-Chart für Kafka (Messaging) sowie `catalog-api`/`frontend`. `notification-worker`
und der CNPG-Postgres-Cluster haben eigene, eigenständige Charts (`charts/notification-worker/`,
`charts/postgres-cluster/`) statt Teil dieses Umbrella-Charts zu sein — siehe `../README.md`.
`reservation-worker` folgt hier als eigenes Template, sobald der Service existiert.

## Struktur

```
ticketflow/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # kleine Replica-/Ressourcenwerte, 1 Kafka-Broker
├── values-prod.yaml         # HA: 3 Kafka-Broker, 3 catalog-api-/frontend-Replicas
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen
└── templates/
    ├── messaging/                         # Strimzi Kafka (KRaft, kein ZooKeeper)
    │   ├── kafka-broker-nodepool.yaml
    │   ├── kafka-controller-nodepool.yaml
    │   ├── kafka-cluster.yaml
    │   ├── kafka-metrics-configmap.yaml   # JMX-Prometheus-Exporter-Regeln (P7)
    │   └── kafka-topics.yaml              # aus .Values.kafka.topics generiert
    ├── catalog-api/
    │   ├── deployment.yaml                # DATABASE_URL aus Secret, Rest aus ConfigMap
    │   ├── service.yaml
    │   ├── configmap.yaml
    │   ├── servicemonitor.yaml            # /metrics, P7
    │   └── poddisruptionbudget.yaml
    ├── frontend/
    │   ├── deployment.yaml                # config.js-Override per ConfigMap-Volume-Mount
    │   ├── service.yaml
    │   ├── configmap.yaml                 # setzt API_BASE_URL="" (Same-Origin via Ingress)
    │   ├── ingress.yaml                   # /events,/reservations -> catalog-api, / -> frontend
    │   └── poddisruptionbudget.yaml
    ├── reservation-worker/   # TODO, sobald der Service existiert
    └── database/, config/, monitoring/    # nicht vorgesehen - CNPG-Cluster und Grafana/
                                            # Alerts liegen in eigenen Charts bzw.
                                            # kube-prometheus-stack, siehe Root-README
```

**Bekannte offene Abhängigkeit:** `catalogApi.databaseSecretName` (Default
`ticketflow-database-url`) erwartet ein Secret mit Key `DATABASE_URL` (fertiger
Postgres-Connection-String) im Namespace `ticketflow`. `charts/postgres-cluster` (Namespace
`application`) liefert aktuell nur ein Secret mit getrennten `username`/`password`-Feldern und
einer DB namens `postgres` statt `ticketflow` — dieses Secret muss noch manuell angelegt oder
`charts/postgres-cluster` entsprechend erweitert werden, bevor `catalog-api` im Cluster wirklich
`/readyz`-grün wird (der Pod crasht dabei nicht, bleibt aber "not ready").

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `kafka.controller/broker.replicas` | 1 | 3 | dev: ein Broker reicht für Funktionstests; prod: Ausfallsicherheit (P5) |
| `catalogApi.replicas` / `frontend.replicas` | 1 | 3 | prod: `topologySpreadConstraints` (P4) verteilen erst ab mehreren Replicas sichtbar über die drei simulierten Regionen |

## Verwendung

```bash
helm lint charts/ticketflow -f charts/ticketflow/values-dev.yaml
helm template ticketflow charts/ticketflow -f charts/ticketflow/values-dev.yaml
```

Deployment läuft indirekt über ArgoCD (`gitops/apps/ticketflow.yaml`, `targetRevision: main`,
Namespace `ticketflow`).
