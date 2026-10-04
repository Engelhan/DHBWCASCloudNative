# Chart: reservation-worker

Kafka-Consumer, der die eigentliche Sitzplatzbuchung durchführt (Carlos' Scope, siehe
`src/reservation-worker/README.md`). Folgt dem gemeinsamen Muster für Applikations-Charts, siehe
`charts/README.md` ## Muster für Applikations-Charts.

## Struktur

```
reservation-worker/
├── Chart.yaml
├── values.yaml              # feste Key-Reihenfolge, s. charts/README.md
├── values-dev.yaml          # 1 Replica, enges KEDA-Fenster, PDB aus
├── values-prod.yaml         # 2 Replicas, KEDA bis 10, PDB an
├── values.schema.json
└── templates/
    ├── _helpers.tpl                   # Namen/Labels/Image
    ├── deployment.yaml
    ├── configmap.yaml
    ├── secret.yaml                    # chart-eigene Postgres-Credentials (s.u.)
    ├── keda-scaledobject.yaml         # P6: Skalierung auf Consumer-Group-Lag
    ├── podmonitor.yaml                # /metrics, P7
    └── poddisruptionbudget.yaml
```

Bewusst **kein** `Service`: den HTTP-Port konsumiert niemand sonst (reiner Kafka-Consumer,
Kubelet-Probes gehen direkt an den Pod), deshalb scrapet ein `PodMonitor` `/metrics` über den
Pod-Label-Selector statt des sonst üblichen `Service`+`ServiceMonitor`-Paars.

## Zentrale Werte

Image-Registry und -Tag kommen aus `gitops/env/cluster.yaml`; der Chart bildet daraus
`<ticketflow.imageRegistry>/ticketflow-reservation-worker:<ticketflow.imageTag>`.
`image.repository`/`.tag` bleiben als Einzelfall-Override möglich, sind aber normalerweise leer.

## Besonderheiten

- **KEDA (P6):** `config.keda.*` steuert das `ScaledObject`. `spec.replicas` des Deployments wird
  von ArgoCD bewusst nicht verglichen (`ignoreReplicas` am Element `reservation-worker` in
  `gitops/applicationsets/`) — sonst würde `selfHeal` jede Skalierung zurücksetzen.
- **Postgres-Zugangsdaten:** kommen direkt aus dem von CNPG selbst erzeugten Secret
  `<ticketflow.postgres.databaseName>-postgres-cluster-app` (Keys `username`/`password`; nicht über
  OpenBao, dort liegt nur eine Audit-Kopie) —
  Datenbank und Anwendung teilen sich seit 2026-09-17 denselben Namespace, ein chart-eigenes
  Duplikat ist nicht mehr nötig. Host und Datenbankname leiten sich aus demselben zentralen Wert
  ab.
- **Kafka-SASL (B2):** `config.kafkaSasl.secretName` referenziert das vom Strimzi User Operator
  automatisch angelegte Secret — nicht von diesem Chart erzeugt.

## values-dev.yaml vs. values-prod.yaml

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `replicaCount` / `config.keda.minReplicaCount` | 1 | 2 | prod hält auch ohne Last zwei Consumer, damit ein Pod-/Node-Ausfall die Verarbeitung nicht bis zum nächsten KEDA-Poll anhält |

Alles andere ist in beiden Profilen gleich, weil der Flash-Sale-Lasttest auf dem dev-Cluster läuft:
KEDA skaliert 1–10 Pods (`maxReplicaCount` 10 bei 20 Partitionen, jeder Pod genau 2), das
PodDisruptionBudget (`maxUnavailable: 1`) ist an — es blockiert auch bei einem einzigen Pod keinen
Node-Drain. Herleitung aller KEDA-Werte als Kommentar in `values.yaml` (`config.keda`).

Welche Datei greift, steuert `ticketflow.environment` in `gitops/env/cluster.yaml`.

## Verwendung

```bash
helm lint charts/reservation-worker \
  -f charts/reservation-worker/values.yaml \
  -f charts/reservation-worker/values-dev.yaml \
  -f gitops/env/cluster.yaml
```

Die zentrale Werte-Datei muss immer mitgegeben werden, sonst bricht der Chart mit einer
`required`-Meldung ab.
