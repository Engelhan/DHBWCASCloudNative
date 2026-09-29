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
    ├── secret.yaml                    # nur falls secret.create=true - Standard seit B1: aus
    ├── externalsecret.yaml            # OpenBao gesynct (siehe ## Secret-Namen und ## OpenBao)
    ├── podmonitor.yaml                # /metrics, P7
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
  `config.smtp.enabled`. Die zugehörigen Secret-Werte stehen nirgends in diesem Repo (siehe ##
  OpenBao).

## OpenBao

Seit B1 (Root-README ## Bonus) liefert `templates/externalsecret.yaml` (`externalSecret.enabled`,
Default `true`) das Secret aus OpenBao statt `templates/secret.yaml` (`secret.create`, jetzt
Default `false`) es chart-eigen mit Platzhaltern zu rendern — Ziel-Name/-Keys sind identisch,
`deployment.yaml` ändert sich nicht. Der Bootstrap-Job in `charts/openbao` seedet die KV-Pfade
`secret/notification-worker/smtp` und `secret/notification-worker/kafka-tls` beim ersten Sync nur
mit leeren Platzhaltern (SMTP/Kafka-mTLS bleiben standardmäßig deaktiviert). Wer die Funktion
tatsächlich aktivieren will, trägt die echten Werte einmalig manuell nach, z. B.:

```bash
kubectl -n openbao exec openbao-0 -- bao kv put secret/notification-worker/smtp \
  username=<smtp-user> password=<smtp-password>
```

Das External Secret synct den neuen Wert innerhalb von `spec.refreshInterval` (1h) automatisch
nach — kein Helm-Redeploy nötig, nur `config.smtp.enabled`/`config.kafkaTls.enabled` auf `true`
setzen.

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

## Secret-Namen

Das chart-eigene Secret heisst **`<release>-credentials`**, nicht `<release>`. Der Suffix ist
Pflicht, kein Stilmittel:

Der Strimzi User Operator legt fuer jede `KafkaUser`-CR ein Secret mit **exakt dem CR-Namen** an.
Die CR dieses Dienstes heisst `notification-worker` (`charts/kafka-cluster/values.yaml`,
`kafka.users`) und liegt im selben Namespace, in den dieser Chart deployt (`ticketflow`). Ohne
Suffix waeren das zwei Controller auf einem Objekt:

- ArgoCD (`selfHeal`) schreibt es mit den SMTP-Platzhaltern zurueck,
- der User Operator schreibt es mit den SCRAM-Credentials zurueck,
- das `envFrom: secretRef` des Deployments zieht dabei mal die SMTP-Keys, mal `password` und
  `sasl.jaas.config` als Env-Variablen in den Container,
- und der `KAFKA_SASL_PASSWORD`-`secretKeyRef` auf `password` laeuft ins Leere, sobald ArgoCD
  zuletzt geschrieben hat - der Dienst kommt dann nicht mehr an Kafka.

Beide Namen entstehen an einer Stelle: `templates/_helpers.tpl`, `notification-worker.secretName`.
`secret.name` in `values.yaml` ueberschreibt ihn, etwa wenn `secret.create: false` gesetzt ist und
ein External Secret das Objekt unter eigenem Namen liefert.

Die uebrigen Charts im `ticketflow`-Namespace haben dasselbe Problem nicht: `reservation-worker`
besitzt kein eigenes gleichnamiges Secret mehr (nur das suffigierte `-keda-auth`), und die
Komponenten von `charts/ticketflow-app` heissen `ticketflow-app-catalog-api`/`-frontend`. **Wer
hier ein neues Secret anlegt, prueft vorher die Namen in `kafka.users`.**
