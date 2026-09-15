# observability/ – Dashboard und Alert-Regel als Code (P7)

Ergänzt die `/metrics`-Endpunkte und `ServiceMonitor`-Ressourcen aus
[`charts/kafka-cluster/templates/monitoring/`](../charts/kafka-cluster/templates/monitoring/) um die
eigentlichen Inhalte:

- [`dashboards/`](dashboards/): Grafana-Dashboard-JSON, wird über eine ConfigMap
  (`grafana_dashboard: "1"`-Label) automatisch vom Grafana-Sidecar des
  kube-prometheus-stack geladen – das Dashboard existiert damit als Code im Repo, nicht als
  manuell in der Grafana-UI geklickter Zustand.
- [`alerts/`](alerts/): `PrometheusRule`-Manifeste für Alertmanager.

## Geplantes Dashboard "TicketFlow Overview"

- Reservierungen/Sekunde, aufgeteilt nach `confirmed`/`rejected`
- Kafka-Consumer-Lag von `reservation-worker` (die KEDA-Scaling-Metrik selbst, sichtbar neben der
  tatsächlichen Replica-Zahl)
- Replica-Anzahl `reservation-worker` über Zeit
- p95-Latenz `catalog-api`
- CNPG-Cluster-Status (Primary/Replica-Zahl, Replikationslag)

## Geplante Alert-Regel

`ReservationBacklogHigh`: feuert, wenn der Kafka-Consumer-Lag von `reservation-worker` über
einen fachlich begründeten Schwellwert (z.B. 500 unverarbeitete Reservierungsanfragen) für mehr
als 2 Minuten anhält – das ist der Punkt, an dem Nutzer spürbar auf ihre Bestätigung warten
müssten. Wird im Screencast durch den Lastgenerator ausgelöst.

`TODO`: Schwellwert nach ersten Lasttests mit echten Zahlen kalibrieren statt eines Schätzwerts.
