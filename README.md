# TicketFlow

Cloud-native Ticketverkaufsplattform für Live-Events. Prüfungsleistung "Cloud Infrastructures and
Cloud Native Applications", DHBW. Dieses Dokument ersetzt den schriftlichen Bericht vollständig.

> **Status:** Repository-Grundgerüst. Diese README beschreibt die geplante und im Aufbau
> befindliche Architektur; Abschnitte, die noch nicht final implementiert sind, sind mit `TODO`
> markiert. Jeder Ordner enthält eine eigene README mit Detailplanung.

## Problem

Beim Vorverkauf populärer Events (Konzert, Stadion-Match) starten tausende Nutzer innerhalb
weniger Sekunden denselben Kaufversuch für eine kleine, feste Anzahl an Plätzen. Ein klassischer
synchroner CRUD-Ansatz (Request → sofortiger DB-Write) macht die Datenbank zum Flaschenhals:
Nutzer sehen Timeouts statt einer fairen Reihenfolge, und Plätze werden im Zweifel doppelt
vergeben (Overselling). TicketFlow trennt die *Annahme* einer Reservierungsanfrage von deren
*Verarbeitung* über einen Message Bus und skaliert die Verarbeitung anhand der Warteschlangenlänge
nach. Genau dieser Lastpfad – "viele gleichzeitige Reservierungen für ein knappes Kontingent" –
ist der fachliche Grund, warum Queue, Datenbank und Autoscaling hier tatsächlich gebraucht werden,
nicht nur vorgeführt.

## Architektur

```mermaid
flowchart TB
    subgraph OpenStack["DHBWCloud / OpenStack"]
        subgraph VMs["VMs (Terraform + Ansible, k3s)"]
            CP["control-plane VM"]
            W1["worker VM 1 (Label: region-a)"]
            W2["worker VM 2 (Label: region-b)"]
            W3["worker VM 3 (Label: region-c)"]
        end
    end

    subgraph Cluster["k3s-Cluster"]
        FE["frontend (Pod)"] --> API
        API["catalog-api (Pods, HPA)"] -->|"REST"| FE
        API -->|"produce ReservationRequested"| KAFKA[("Kafka - Strimzi\n(StatefulSet, 3 Broker)")]
        KAFKA -->|"consume"| WORKER["reservation-worker (Pods, KEDA-scaled)"]
        WORKER -->|"seat lock / commit"| PG[("PostgreSQL - CNPG\n(Primary + 2 Replicas, PVC)")]
        WORKER -->|"produce ReservationConfirmed/Rejected"| KAFKA
        KAFKA -->|"consume"| NOTIFY["notification-worker (Pods)"]
        API -->|"read seat availability"| PG

        PROM["Prometheus"] -.scrape /metrics.-> API
        PROM -.scrape /metrics.-> WORKER
        PROM -.scrape /metrics.-> NOTIFY
        PROM --> GRAFANA["Grafana Dashboard (as Code)"]
        PROM --> ALERT["Alertmanager (Alert-Regel: Backlog/Fehlerquote)"]
        ARGOCD["ArgoCD"] -.auto-sync.-> Cluster
    end

    LOADGEN["load-generator (k6)"] -->|"künstliche Lastspitze"| API

    W1 -. "Topology-Spread" .-> PG
    W2 -. "Topology-Spread" .-> PG
    W3 -. "Topology-Spread" .-> PG
    W1 -. "Topology-Spread" .-> KAFKA
    W2 -. "Topology-Spread" .-> KAFKA
    W3 -. "Topology-Spread" .-> KAFKA
```

**Datenfluss von der VM bis zum Pod:**

1. Terraform/OpenTofu erzeugt gegen den OpenStack-Provider der DHBWCloud vier VMs (1
   Control-Plane, 3 Worker) direkt am vorhandenen, extern routbaren Projektnetz `DHBWV6` – kein
   eigenes Netz, kein Router, keine selbst verwaltete Security Group — siehe
   [`infra/terraform/`](infra/terraform/). Jeder Worker erhält eine virtuell simulierte
   Regions-Metadata (`region-a`/`region-b`/`region-c`), da die DHBWCloud real nur eine
   Region/AZ bereitstellt.
2. Ansible konfiguriert die VMs, installiert k3s (Server auf der Control-Plane, Agents auf den
   Workern), übernimmt die simulierte Region als Kubernetes-Node-Label
   (`topology.ticketflow/region`) und bootstrapt ArgoCD — siehe [`infra/ansible/`](infra/ansible/).
3. ArgoCD übernimmt ab hier per GitOps (App-of-Apps) den Rest: CNPG-Operator, Strimzi-Operator,
   KEDA, kube-prometheus-stack und den TicketFlow-Helm-Chart selbst — siehe
   [`gitops/`](gitops/) und [`charts/ticketflow/`](charts/ticketflow/).
4. `catalog-api` nimmt Reservierungsanfragen entgegen und veröffentlicht sie als Event auf Kafka,
   statt synchron in die Datenbank zu schreiben.
5. `reservation-worker` konsumiert diese Events, sperrt den Sitzplatz transaktional in CNPG-Postgres
   und veröffentlicht das Ergebnis (bestätigt/abgelehnt) wieder auf Kafka.
6. `notification-worker` konsumiert das Ergebnis und simuliert eine Bestätigungs-Benachrichtigung.
7. Der `load-generator` erzeugt künstlich die namensgebende Lastspitze; Prometheus/Grafana machen
   Backlog, Fehlerquote und Replica-Anzahl sichtbar, KEDA reagiert auf den Kafka-Consumer-Lag.

Detailskizze und Screenshots des tatsächlichen Deployments folgen in
[`docs/architecture/`](docs/architecture/) (`TODO`, sobald der Cluster steht).

## Betrieb

> `TODO`: Sobald Terraform/Ansible/Charts final sind, exakte, kopierbare Befehle inkl. Beispielwerten
> hier verifizieren. Geplanter Ablauf:

```bash
# 1. Infrastruktur auf der DHBWCloud erzeugen
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # OpenStack-Credentials/Projekt eintragen
terraform init
terraform apply

# 2. VMs konfigurieren, k3s-Cluster aufbauen, ArgoCD bootstrappen
cd ../ansible
ansible-inventory -i inventory/hosts.ini --list        # von Terraform-Output generiert
ansible-playbook -i inventory/hosts.ini playbooks/site.yml

# 3. Restliche Plattform per GitOps ausrollen (Operatoren, Kafka, Postgres, App)
export KUBECONFIG=./kubeconfig   # von Ansible aus der Control-Plane geholt
kubectl apply -f gitops/apps/app-of-apps.yaml

# 4. Rollout prüfen
kubectl get pods -A -o wide
kubectl get kafka,cluster.postgresql.cnpg.io -A

# 5. Künstlichen Lastpfad auslösen
cd ../../src/load-generator
k6 run flash-sale.js --env EVENT_ID=demo-event --env VUS=500

# 6. Beobachten
kubectl port-forward svc/kube-prometheus-stack-grafana 3000:80 -n monitoring
# -> Dashboard "TicketFlow Overview" öffnen
```

Ein frisch erzeugtes Projekt auf der DHBWCloud muss allein mit diesen Schritten (ohne manuelle
Portal-Klicks) zum laufenden Stack führen.

## Entscheidungen

Je Pflicht- und Wahlthema: Wahl, Alternative, Begründung.

**P1 – Anwendung und Domäne.** *Wahl:* drei eigene Services (`catalog-api`,
`reservation-worker`, `notification-worker`) plus CNPG-Postgres und Strimzi-Kafka als
zustandsbehaftete Dienste. *Alternative:* ein einzelner Monolith mit synchronem DB-Zugriff.
*Begründung:* Erst die Trennung von Annahme (API) und Verarbeitung (Worker) über eine Queue macht
den Lastpfad des Flash-Sales beherrschbar und erlaubt, genau den Engpass-Service unabhängig zu
skalieren.

**P2 – Infrastruktur als Code.** *Wahl:* Terraform/OpenTofu gegen den OpenStack-Provider für
VMs/Netz, Ansible für OS-Konfiguration und k3s-Bootstrap. *Alternative:* Managed Kubernetes oder
manuelle VM-Erstellung im Portal. *Begründung:* Managed K8s ist auf der DHBWCloud ausgeschlossen;
Terraform+Ansible ist die vorgegebene Kombination und trennt sauber "Ressourcen existieren" von
"Ressourcen sind konfiguriert".

**P3 – Deployment und Konfiguration.** *Wahl:* Helm-Werte legen Requests/Limits je Service fest,
Images werden mit Git-SHA getaggt, DB-Zugangsdaten/Kafka-Bootstrap-Server kommen aus
ConfigMap/Secret. *Alternative:* Werte im Image hart kodieren. *Begründung:* Ohne getrennte
Konfiguration könnten dev/prod-Werte (siehe W1) nicht dasselbe Image nutzen, und Secrets im Image
wären ein Sicherheitsproblem.

**P4 – Scheduling und Platzierung.** *Wahl:* Jeder Worker trägt ein Node-Label
`topology.ticketflow/region` (`region-a`/`region-b`/`region-c`, von Terraform als Metadata
vergeben und von Ansible in ein Kubernetes-Label übersetzt – die DHBWCloud stellt real nur eine
Region/AZ bereit, die Verteilung wird für diese Entscheidung bewusst simuliert). CNPG-Postgres
(Primary+2 Replicas) und die Kafka-Broker (Strimzi, 3 Broker) erhalten je eine
`topologySpreadConstraint`/`podAntiAffinity` auf diesem Label, sodass ihre Replicas nie auf
demselben simulierten Standort landen. *Alternative:* freie Platzierung durch den
Standard-Scheduler, oder ein einzelner dedizierter "Storage-Node" für beide Dienste.
*Begründung:* Ticket-Reservierungen und deren Event-Log dürfen nicht durch den Ausfall eines
einzelnen Nodes komplett verschwinden; ein einzelner Storage-Node wäre selbst ein Single Point of
Failure und hätte die Platzierungsfrage nur verschoben statt aus der Domäne (Ausfallsicherheit
beim Ticketverkauf) beantwortet.

**P5 – Resilienz und Persistenz.** *Wahl:* Liveness/Readiness-Probes auf allen drei Services, CNPG
verwaltet PVCs für Primary+2 Replicas, Strimzi-Kafka-Broker laufen als StatefulSet mit PVC,
PodDisruptionBudgets sichern Rolling Updates ab. *Alternative:* `emptyDir` oder ein Single-Instance-
Postgres ohne Replikation. *Begründung:* Ticket-Reservierungen sind Geschäftsdaten; ein Pod-Verlust
darf keine Bestellung verlieren, ein Rolling Update der API darf keine Downtime verursachen.

**P6 – Skalierbarkeit.** *Wahl:* KEDA skaliert `reservation-worker` auf Basis des
Kafka-Consumer-Group-Lags. *Alternative:* HPA auf CPU-Auslastung. *Begründung:* Die eigentliche
fachliche Engpassgröße ist "wie viele Reservierungen warten auf Verarbeitung", nicht CPU-Last –
CPU kann bei I/O-gebundener Arbeit trügerisch niedrig bleiben, obwohl die Warteschlange wächst.

**P7 – Observability.** *Wahl:* Jeder Service exponiert `/metrics` (Prometheus-Client), CNPG und
Strimzi bringen eigene ServiceMonitor-fähige Metriken mit, ein Grafana-Dashboard liegt als
Code (`observability/dashboards/`) vor, eine Alert-Regel feuert auf
"Reservation-Backlog über Schwellwert für 5 Minuten". *Alternative:* Ad-hoc-Logging ohne
Metriken. *Begründung:* Ohne Metriken lässt sich P6 nicht demonstrieren – die Alert-Regel macht
sichtbar, ab wann die Lastspitze geschäftlich relevant wird, nicht nur technisch.

**W1 – Packaging (Helm).** *Wahl:* ein Umbrella-Chart `charts/ticketflow` mit
`values-dev.yaml`, `values-prod.yaml` und `values.schema.json`. *Alternative:* Kustomize mit
Base+Overlays. *Begründung:* Ein Chart, das CNPG-`Cluster`- und Strimzi-`Kafka`-Custom-Resources
mitsamt den drei Anwendungs-Deployments bündelt, macht die Differenz zwischen dev (kleine
Replica-Zahlen, ein Broker) und prod (HA-Setup) explizit und typgeprüft über das Schema.

**W2 – GitOps.** *Wahl:* ArgoCD mit App-of-Apps-Pattern, Auto-Sync und Self-Heal.
*Alternative:* reines `kubectl apply`/CI-Pipeline ohne Cluster-seitigen Agenten.
*Begründung:* Ansible bootstrappt bewusst nur so viel, dass ArgoCD läuft; alles danach (Operatoren,
Kafka, Postgres, App) kommt aus Git – Drift (z.B. manuell veränderte Replica-Zahl) wird
nachweisbar automatisch zurückgedreht.

*(TODO: nach Implementierung um Verweise auf konkrete Commits/Dateien ergänzen, wo das jeweils
umgesetzt ist.)*

## Grenzen

- Die DHBWCloud stellt nur eine begrenzte Anzahl VMs/Ressourcen bereit; CNPG- und Kafka-Replikation
  sind daher klein dimensioniert (nicht produktionsreif für echte Lastspitzen mit Millionen Nutzern).
- Zahlungsabwicklung ist simuliert (kein echtes Payment-Gateway); `notification-worker` loggt nur,
  statt E-Mail/SMS wirklich zu versenden.
- k3s statt eines vollwertigen Kubernetes verkleinert den Kontrollebenen-Fußabdruck, verhält sich
  aber nicht in jedem Detail wie ein Cloud-Provider-Kubernetes.
- Der Lastgenerator bildet einen synthetischen "alle klicken gleichzeitig"-Fall nach; reales
  Nutzerverhalten (Netzwerklatenz-Verteilung, Retry-Verhalten von Browsern) ist nicht modelliert.
- Ein Node-Ausfall der Control-Plane ist nicht automatisch behoben (kein HA-Control-Plane geplant);
  das ist eine bewusste Kosten/Nutzen-Entscheidung für den Laborrahmen.

*(TODO: nach Lasttests unter echten Bedingungen konkretisieren, was tatsächlich beobachtet wurde.)*
