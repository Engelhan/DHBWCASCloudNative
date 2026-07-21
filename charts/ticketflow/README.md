# Chart: ticketflow

Umbrella-Helm-Chart für die gesamte TicketFlow-Anwendungsebene.

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
    ├── notification-worker/ # Deployment, ServiceMonitor
    ├── frontend/             # Deployment, Service, Ingress
    ├── database/
    │   └── cnpg-cluster.yaml # CNPG `Cluster`-CR (PVC-Größe, Instanzanzahl aus values)
    ├── messaging/
    │   └── kafka-cluster.yaml # Strimzi `Kafka`- und `KafkaTopic`-CRs
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

`TODO`: Templates schreiben, sobald die drei Services ein `/metrics`- und `/healthz`-Endpoint
haben und die CNPG-/Strimzi-Operator-CRD-Versionen (aus `gitops/apps/`) feststehen.
