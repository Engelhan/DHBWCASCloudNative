# Chart: notification-worker

Zweite, unabhängige Kafka-Consumer-Group auf `RESULT_TOPIC` (Niklas' Scope, siehe
`src/notification-worker/README.md`) — belegt, dass Kafka hier als Bus mit Fan-out genutzt wird,
nicht als Punkt-zu-Punkt-Queue. Folgt dem gemeinsamen Muster für Applikations-Charts, siehe
`charts/README.md` ## Muster für Applikations-Charts.

## Struktur

```
notification-worker/
├── Chart.yaml
├── values.yaml
├── values-dev.yaml          # 1 Replica, PDB minAvailable 1
├── values-prod.yaml         # 3 Replicas, PDB minAvailable 2
├── values.schema.json
└── templates/
    ├── _helpers.tpl
    ├── deployment.yaml
    ├── configmap.yaml
    ├── secret.yaml                    # SMTP-/Kafka-TLS-Material, default leer
    ├── podmonitor.yaml                # /metrics, P7
    ├── networkpolicy.yaml             # default aus
    └── poddisruptionbudget.yaml
```

Bewusst **kein** `Service` — gleiche Begründung wie bei `charts/reservation-worker`.

## Zentrale Werte

Image-Registry und -Tag kommen aus `gitops/env/cluster.yaml`; der Chart bildet daraus
`<ticketflow.imageRegistry>/ticketflow-notification-worker:<ticketflow.imageTag>`. Vorher war
`image.tag` hier leer und ein Deploy deshalb ohne `--set` gar nicht möglich.

## Besonderheiten

- **Zustellsemantik:** der Kafka-Offset wird erst nach erfolgreichem Versand committet; eine
  Nachricht, die `config.notifyMaxRetries` erschöpft, geht auf das optionale Dead-Letter-Topic
  (`config.dltTopic`), bleibt aber uncommittet — bewusste At-least-once-Abwägung, siehe
  Package-Doku von `internal/kafkaconsumer`.
- **Benachrichtigungskanal:** `LogNotifier` (Default, demo-tauglich) oder `SMTPNotifier` über
  `config.smtp.enabled`. Die zugehörigen Secret-Felder sind absichtlich leer und werden beim
  Deploy gesetzt, nie committet.

## values-dev.yaml vs. values-prod.yaml

| Wert | dev | prod | Begründung |
|---|---|---|---|
| `replicaCount` | 1 | 3 | prod verteilt die Partitionen des Result-Topics über drei Consumer und gibt `topologySpreadConstraints` (P4) etwas zu verteilen |
| `podDisruptionBudget.minAvailable` | 1 | 2 | bei einem einzigen Replica würde ein Budget von 2 jeden Node-Drain blockieren |

Welche Datei greift, steuert `ticketflow.environment` in `gitops/env/cluster.yaml`.

## Verwendung

```bash
helm lint charts/notification-worker \
  -f charts/notification-worker/values.yaml \
  -f charts/notification-worker/values-dev.yaml \
  -f gitops/env/cluster.yaml
```
