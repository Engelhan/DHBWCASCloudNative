# Chart: ticketflow

Umbrella-Helm-Chart für `catalog-api`/`frontend`. `notification-worker`, der CNPG-Postgres-Cluster
und der Kafka-Cluster haben eigene, eigenständige Charts (`charts/notification-worker/`,
`charts/postgres-cluster/`, `charts/kafka-cluster/`) statt Teil dieses Umbrella-Charts zu sein —
siehe `../README.md`. Kafka/Apicurio-Schema-Registrierung lagen ursprünglich hier im selben
Helm-Release wie `catalog-api`/`frontend`, wurden aber am 2026-09-15 nach `charts/kafka-cluster/`
ausgelagert (unabhängige Deploybarkeit, analog zu `charts/postgres-cluster/` vs.
`charts/cnpg-operator/`). `reservation-worker` folgt hier als eigenes Template, sobald der Service
existiert.

## Struktur

```
ticketflow/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # kleine Replica-/Ressourcenwerte
├── values-prod.yaml         # HA: 3 catalog-api-/frontend-Replicas
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen
└── templates/
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

**Bekannte offene Abhängigkeit:** `catalog-api` baut den Connection-String selbst aus
`catalogApi.database.{host,port,name,sslMode}` (ConfigMap) + `catalogApi.database.
credentialsSecretName` (Secret, Keys `username`/`password`) zusammen — Team-Entscheidung, siehe
`src/catalog-api/README.md` ## Konfiguration. Das Secret muss aber im **selben Namespace**
(`ticketflow`) liegen wie `catalog-api` (Kubernetes erlaubt kein `secretKeyRef` über
Namespace-Grenzen), `charts/postgres-cluster` legt sein Secret aber in Namespace `application`
an. `scripts/copy-database-credentials-secret.sh` kopiert es unverändert herüber, bis das
Team-seitig sauberer gelöst ist (z.B. External Secret/Reflector). DB-Name ist aktuell `postgres`
(Default von `charts/postgres-cluster`), nicht `ticketflow`.

## values-dev.yaml vs. values-prod.yaml — worin sie sich unterscheiden

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `catalogApi.replicas` / `frontend.replicas` | 1 | 3 | prod: `topologySpreadConstraints` (P4) verteilen erst ab mehreren Replicas sichtbar über die drei simulierten Regionen |

(Kafka-Werte: siehe `charts/kafka-cluster/README.md` ## values-dev.yaml vs. values-prod.yaml.)

## Verwendung

```bash
helm lint charts/ticketflow -f charts/ticketflow/values-dev.yaml
helm template ticketflow charts/ticketflow -f charts/ticketflow/values-dev.yaml
```

Deployment läuft indirekt über ArgoCD (`gitops/apps/ticketflow.yaml`, `targetRevision: main`,
Namespace `ticketflow`).
