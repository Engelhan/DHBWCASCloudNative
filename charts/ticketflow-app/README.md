# Chart: ticketflow-app

Eigenständiger Chart für `catalog-api` + `frontend` (Philipps Scope). Bewusst NICHT Teil von
`charts/ticketflow` (Kafka + Schema-Registry, Carlos' Scope) — Team-Entscheidung, catalog-api/
frontend nicht in derselben Application wie Kafka deployen, siehe
`gitops/apps/ticketflow-app.yaml`. catalog-api und frontend teilen sich hier einen Chart (statt
je einen eigenen wie `charts/reservation-worker`/`charts/notification-worker`), da frontend
ausschließlich mit catalog-api spricht und sich mit ihm eine Ingress-Route/Origin teilt.

## Struktur

```
ticketflow-app/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte
├── values-dev.yaml          # 1 Replica je Service
├── values-prod.yaml         # 3 Replicas je Service (P4/P5)
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen
└── templates/
    ├── catalog-api/
    │   ├── deployment.yaml                # DB_USER/DB_PASSWORD aus Secret, Rest aus ConfigMap
    │   ├── service.yaml
    │   ├── configmap.yaml
    │   ├── servicemonitor.yaml            # /metrics, P7
    │   └── poddisruptionbudget.yaml
    └── frontend/
        ├── deployment.yaml                # config.js-Override per ConfigMap-Volume-Mount
        ├── service.yaml
        ├── configmap.yaml                 # setzt API_BASE_URL="" (Same-Origin via Ingress)
        ├── ingress.yaml                   # /events,/reservations -> catalog-api, / -> frontend
        └── poddisruptionbudget.yaml
```

**Schnittstelle zu `charts/ticketflow` (Kafka):** `catalogApi.env.kafkaBrokers` referenziert den
Kafka-Bootstrap-Service aus diesem anderen Chart/derselben Application-Gruppe per fest
verdrahtetem Service-DNS-Namen (`ticketflow-kafka-kafka-bootstrap.ticketflow.svc:9092`) — beide
Charts laufen im selben Kubernetes-Namespace (`ticketflow`), auch wenn sie getrennte
Helm-Releases/ArgoCD-Applications sind. Ändert sich der Kafka-Cluster-Name in
`charts/ticketflow/values.yaml`, muss dieser Wert hier manuell nachgezogen werden (kein
`--set-file`/Cross-Chart-Referencing).

**Bekannte offene Abhängigkeit:** `catalog-api` baut den Postgres-Connection-String selbst aus
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

## Verwendung

```bash
helm lint charts/ticketflow-app -f charts/ticketflow-app/values-dev.yaml
helm template ticketflow-app charts/ticketflow-app -f charts/ticketflow-app/values-dev.yaml
```

Deployment läuft indirekt über ArgoCD (`gitops/apps/ticketflow-app.yaml`, `targetRevision: main`,
Namespace `ticketflow`).
