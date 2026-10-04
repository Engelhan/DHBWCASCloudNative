# Chart: ticketflow-app

Eigenständiger Chart für `catalog-api` + `frontend` (Philipps Scope). Bewusst NICHT Teil von
`charts/kafka-cluster` (Kafka + Schema-Registry, Carlos' Scope) — Team-Entscheidung, catalog-api/
frontend nicht in derselben Application wie Kafka deployen. catalog-api und frontend teilen sich
hier einen Chart (statt je einen eigenen wie `charts/reservation-worker`/
`charts/notification-worker`), da frontend ausschließlich mit catalog-api spricht und sich mit ihm
eine Ingress-Route/Origin teilt. Innerhalb des Charts folgt aber **jede der beiden Komponenten
exakt demselben Muster** wie die Worker-Charts — siehe `charts/README.md` ## Muster für Applikations-Charts.

## Struktur

```
ticketflow-app/
├── Chart.yaml
├── values.yaml              # gemeinsame Basiswerte, feste Key-Reihenfolge je Komponente
├── values-dev.yaml          # 2 Replicas je Service
├── values-prod.yaml         # 3 Replicas je Service + minAvailable: 2 (P4/P5)
├── values.schema.json       # JSON-Schema: Pflichtfelder, Typen, verbietet image.tag "latest"
└── templates/
    ├── _helpers.tpl                          # Namen/Labels/Image, auch komponentenbezogen
    ├── catalog-api/
    │   ├── deployment.yaml                   # DB_USER/DB_PASSWORD aus dem CNPG-Secret, Rest aus ConfigMap
    │   ├── service.yaml
    │   ├── configmap.yaml
    │   ├── servicemonitor.yaml               # aus (kein Prometheus im Cluster)
    │   └── poddisruptionbudget.yaml
    └── frontend/
        ├── deployment.yaml                   # config.js-Override per ConfigMap-Volume-Mount
        ├── service.yaml
        ├── configmap.yaml                    # setzt API_BASE_URL="" (Same-Origin via Ingress)
        ├── ingress.yaml                      # /events,/reservations -> catalog-api, / -> frontend
        └── poddisruptionbudget.yaml
```

Objektnamen und Labels kommen ausschließlich aus `_helpers.tpl`; jede Ressource trägt zusätzlich
`app.kubernetes.io/component: catalog-api|frontend`. Einen `namespace:`-Wert gibt es nicht mehr —
den bestimmt die ArgoCD-Application (`destination.namespace` in `gitops/applicationsets/`).

## Zentrale Werte

Image-Registry, Image-Tag und DNS-Zone stehen **nicht** in diesem Chart, sondern in
`gitops/env/cluster.yaml`. Der Chart bildet daraus:

| Wert | Ableitung |
|---|---|
| catalog-api-Image | `<ticketflow.imageRegistry>/ticketflow-catalog-api:<ticketflow.imageTag>` |
| frontend-Image | `<ticketflow.imageRegistry>/ticketflow-frontend:<ticketflow.imageTag>` |
| Ingress-Host | `ticketflow.<ticketflow.dnsZone>` |

`catalogApi.image.repository`/`.tag` (und das Gegenstück beim frontend) bleiben als
Einzelfall-Override möglich, sind aber normalerweise leer.

**Schnittstelle zu `charts/kafka-cluster` (Kafka):** Kafka läuft in einem **eigenen Namespace**
(`kafka`), getrennt von dieser Anwendung. `catalogApi.config.kafkaBrokers` ist deshalb leer und
die Adresse wird abgeleitet: `<ticketflow.kafka.clusterName>-kafka-bootstrap.<ticketflow.namespaces.kafka>.svc:9092`.
Cluster-Name und Namespace stehen einmalig in `gitops/env/cluster.yaml` — eine Umbenennung zieht
damit überall durch, statt in vier Charts einzeln nachgetragen werden zu müssen. Das
SASL-`Secret` erreicht catalog-api trotz der Namespace-Grenze, weil `charts/kafka-cluster` seine
`KafkaUser`-CRs bewusst in diesen Namespace rendert (siehe dort `README.md` ## Namespaces).

**Datenbank-Zugangsdaten:** `catalog-api` baut den Postgres-Connection-String selbst aus
`catalogApi.database.{host,port,name,sslMode}` (ConfigMap) + einem Secret (Keys `username`/
`password`) zusammen — Team-Entscheidung, siehe `src/catalog-api/README.md` ## Konfiguration.
`charts/postgres-cluster` läuft seit 2026-09-17 im **selben Namespace** wie dieser Chart, deshalb
wird das von CNPG selbst erzeugte Secret direkt referenziert statt dupliziert:
`database.credentialsSecretName` ist leer und wird zu
`<ticketflow.postgres.databaseName>-postgres-cluster-app` aufgelöst (bewusst nicht über OpenBao —
die Kopie dort ist nur Audit, siehe `charts/postgres-cluster`), `database.host` zu
`<...>-postgres-cluster-rw`.

## values-dev.yaml vs. values-prod.yaml

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `catalogApi.replicaCount` / `frontend.replicaCount` | 2 | 3 | beide Profile: mindestens zwei Replicas, hart auf verschiedene Regionen verteilt (P4); prod nutzt alle drei Regionen |
| `podDisruptionBudget.minAvailable` | 1 | 2 | lässt in beiden Profilen einen Node-Drain/Rollout-Schritt zu |

Welche der beiden Dateien greift, steuert `ticketflow.environment` in `gitops/env/cluster.yaml` —
nicht die Application, siehe `gitops/applicationsets/`.

## Verwendung

Die zentrale Werte-Datei muss beim lokalen Rendern immer mitgegeben werden, sonst bricht der Chart
mit einer `required`-Meldung ab (statt still falsche Defaults zu nehmen):

```bash
helm lint charts/ticketflow-app \
  -f charts/ticketflow-app/values.yaml \
  -f charts/ticketflow-app/values-dev.yaml \
  -f gitops/env/cluster.yaml

helm template ticketflow-app charts/ticketflow-app \
  -f charts/ticketflow-app/values.yaml \
  -f charts/ticketflow-app/values-dev.yaml \
  -f gitops/env/cluster.yaml
```

Deployment läuft indirekt über ArgoCD (`gitops/applicationsets/`, Element `ticketflow-app`,
Namespace `ticketflow` — Kafka liegt getrennt davon in `kafka`, siehe `charts/README.md` ## Namespace-Aufteilung).
