# Chart: reservation-worker

Kafka-Consumer, der die eigentliche Sitzplatzbuchung durchführt (Carlos' Scope, siehe
`src/reservation-worker/README.md`). Folgt dem gemeinsamen Muster für Applikations-Charts, siehe
`charts/README.md` ## Muster für Applikations-Charts.

## Struktur

```
reservation-worker/
├── Chart.yaml
├── values.yaml              # feste Key-Reihenfolge, s. charts/README.md
├── values-dev.yaml          # keine Abweichungen (Lasttest läuft auf dev)
├── values-prod.yaml         # derzeit ebenfalls keine Abweichungen
├── values.schema.json
└── templates/
    ├── _helpers.tpl                   # Namen/Labels/Image
    ├── deployment.yaml
    ├── configmap.yaml
    ├── keda-scaledobject.yaml         # B2: Skalierung auf Consumer-Group-Lag
    ├── podmonitor.yaml                # aus (kein Prometheus im Cluster)
    └── poddisruptionbudget.yaml
```

Bewusst **kein** `Service`: den HTTP-Port konsumiert niemand sonst (reiner Kafka-Consumer,
Kubelet-Probes gehen direkt an den Pod), deshalb wäre ein `PodMonitor` statt des üblichen
`Service`+`ServiceMonitor`-Paars vorgesehen – er ist aus, solange kein Prometheus im Cluster läuft.

## Zentrale Werte

Image-Registry und -Tag kommen aus `gitops/env/cluster.yaml`; der Chart bildet daraus
`<ticketflow.imageRegistry>/ticketflow-reservation-worker:<ticketflow.imageTag>`.
`image.repository`/`.tag` bleiben als Einzelfall-Override möglich, sind aber normalerweise leer.

## Besonderheiten

- **KEDA (B2):** `config.keda.*` steuert das `ScaledObject` (2–10 Pods, hart auf Regionen verteilt). `spec.replicas` des Deployments wird
  von ArgoCD bewusst nicht verglichen (`ignoreReplicas` am Element `reservation-worker` in
  `gitops/applicationsets/`) — sonst würde `selfHeal` jede Skalierung zurücksetzen.
- **Postgres-Zugangsdaten:** kommen direkt aus dem von CNPG selbst erzeugten Secret
  `<ticketflow.postgres.databaseName>-postgres-cluster-app` (Keys `username`/`password`; nicht über
  OpenBao, dort liegt nur eine Audit-Kopie) — Datenbank und Anwendung teilen sich denselben
  Namespace. Host und Datenbankname leiten sich aus demselben zentralen Wert ab.
- **Kafka-SASL (B4):** `config.kafkaSasl.secretName` referenziert das vom Strimzi User Operator
  automatisch angelegte Secret — nicht von diesem Chart erzeugt.

## values-dev.yaml vs. values-prod.yaml

Beide Profile sind derzeit gleich, weil der Flash-Sale-Lasttest auf dem dev-Cluster läuft: KEDA
skaliert 2–10 Pods (`minReplicaCount` 2 für zwei Regionen auch ohne Last, `maxReplicaCount` 10 bei
20 Partitionen, jeder Pod genau 2), das PodDisruptionBudget (`maxUnavailable: 1`) ist an.
Herleitung aller KEDA-Werte als Kommentar in `values.yaml` (`config.keda`).

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
