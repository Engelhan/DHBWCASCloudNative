# Laborbericht Teil 1 (Sturm)


## Architekturreflexion

TicketFlow ist eine Ticketverkaufsplattform für Live-Events. Der kritische Lastpfad entsteht bei Flash Sales, wenn sehr viele Nutzerinnen und Nutzer innerhalb weniger Sekunden eine begrenzte Zahl an Sitzplätzen reservieren wollen. Dafür nutzt TicketFlow eine cloud-native Architektur. `catalog-api` nimmt Reservierungsanfragen entgegen und schreibt nicht direkt in die Datenbank, sondern publiziert ein `ReservationRequested`-Event auf Kafka. `reservation-worker` verarbeitet die Anfrage asynchron, belegt den Sitzplatz in einer PostgreSQL-Transaktion per Compare-and-Swap (`UPDATE … WHERE status = 'free'`, vgl. PostgreSQL Global Development Group, 2026) und veröffentlicht anschließend `ReservationConfirmed` oder `ReservationRejected`. `notification-worker` verarbeitet die Ergebnis-Events unabhängig weiter.

Ein erster Vorteil ist die elastische Skalierung. Die eigentliche Arbeit liegt nicht in der HTTP-Annahme, sondern in der Verarbeitung und Konfliktbehandlung im Worker. Daher kann `reservation-worker` separat skaliert werden: KEDA skaliert ihn anhand des Kafka-Consumer-Lags von 2 bis 10 Pods (KEDA Authors, o. J.). So bleibt bei einer Lastspitze die API arbeitsfähig, da die Verarbeitung schrittweise aufgeholt wird.

Ein zweiter Vorteil ist die Entkopplung. Kafka puffert Anfragen in einem persistenten, replizierten Log (Kreps et al., 2011) und trennt Frontend, API, Verarbeitung und Benachrichtigung voneinander. Fällt ein Worker temporär aus, gehen noch nicht verarbeitete Events nicht verloren. Das ist für TicketFlow relevant, weil ein Flash Sale nicht an einem kurzfristigen Ausfall der Verarbeitungsschicht scheitern soll.

Dem stehen jedoch klare Nachteile gegenüber. Erstens steigt die Betriebs- und Architekturkomplexität deutlich: Kubernetes, Kafka, PostgreSQL/CNPG, KEDA, ArgoCD und OpenBao müssen zusammenspielen. Allein das GitOps-Setup umfasst 21 ArgoCD-Applications, obwohl die fachliche Anwendung nur aus wenigen Services besteht. Zweitens wird die Logik schwerer nachvollziehbar, weil der Ablauf nicht mehr in einem synchronen Workflow endet, sondern über Events, Statuswerte und Wiederholungen verläuft. Der Nutzer erhält „bestätigt" oder „abgelehnt" nicht direkt, sondern das Frontend fragt alle 2 Sekunden den Status ab, bis der Worker die Reservierung geschrieben hat. Drittens bleibt die Datenbank der fachliche Engpass, weil jeder Sitzplatz nur einmal vergeben werden darf. Kafka glättet nur die Schreiblast (Load Leveling). Die Lesezugriffe der `catalog-api` für die Vorprüfung des Sitzplatzes sind dagegen unkritisch: Sie laufen über den `-ro`-Service auf Read-Only-Replicas, die sich horizontal skalieren lassen und den Primary nicht belasten.

Eine realistische Alternative wäre ein Monolith auf einer einzelnen VM mit direktem Datenbank-Write. Das wäre einfacher zu entwickeln, zu testen und zu betreiben und für eine kleine, stabile Anwendung ausreichend. Bei TicketFlow würden Lastspitzen jedoch ungepuffert als tausende gleichzeitige Transaktionen auf die Datenbank durchschlagen, mit Timeouts und erschöpften Verbindungen als Folge. Die gewählte Lösung nutzt zusätzlich `seat_id` als Partition Key: Alle Anfragen für denselben Sitz landen in derselben Partition und werden nacheinander verarbeitet, sodass konkurrierende Zugriffe auf dieselbe Sitzplatz-Zeile kaum auftreten.

Eine zweite Alternative wäre eine klassische Drei-Schichten-Anwendung auf mehreren VMs hinter einem Load Balancer. Das verbessert die Skalierung der Web- und API-Schicht, bleibt aber bei der Reservierungsverarbeitung synchron und zentral. Die gewählte Lösung passt besser zum Lastfall, ist aber anspruchsvoller und teurer. Unverhältnismäßig wird sie dort, wo die Last nicht spitzenförmig ist: Für einen kleinen Veranstalter mit wenigen hundert Tickets wären vier VMs, Kafka, ein Operator-Stack und 21 Applications überdimensioniert. Auch in unserer Umsetzung läuft die Plattform auf vier VMs der DHBWCloud. Die Ausfallsicherheit durch „Regionen" ist nur simuliert (siehe Dynamic Scheduling).

Die Entscheidung für Cloud Native ist damit vor allem für den fachlichen Lastverlauf vorteilhaft. Stateless Komponenten wie `catalog-api` und `notification-worker` lassen sich horizontal skalieren, während die kritische Reservierungslogik isoliert im `reservation-worker` liegt. Erkauft wird das mit deutlich höherem Betriebsaufwand.


## Cloud-Native Patterns

Die folgenden zwei Muster orientieren sich an der [Patterns Library](https://www.cnpatterns.org/patterns-library): [Microservices Architecture](https://www.cnpatterns.org/development-design/microservices-architecture) (Development & Design) und [Dynamic Scheduling](https://www.cnpatterns.org/infrastructure-cloud/dynamic-scheduling) (Infrastructure & Cloud).

**Microservices Architecture** findet sich in der Aufteilung von TicketFlow wieder: `catalog-api`, `reservation-worker`, `notification-worker` und `frontend` sind getrennte, einzeln deploybare Komponenten mit klaren Aufgaben, die nur über HTTP bzw. Kafka-Events kommunizieren. Das Muster löst in der Domäne das Problem unterschiedlicher Last- und Ausfallprofile: Beim Flash Sale muss nur der Worker stark skalieren, und die Benachrichtigung kann als eigene Consumer-Group ausfallen oder nachlaufen, ohne Reservierungen zu blockieren. Die Pufferung selbst leistet dabei Kafka, nicht der Service-Schnitt. Eine realistische Alternative wäre ein modularer Monolith mit derselben Fachlogik in einem Prozess. Das wäre einfacher zu betreiben, aber nur als Ganzes skalierbar. Der Trade-off besteht im höheren Verteilungsaufwand: Schnittstellen müssen versioniert werden (bei uns über JSON-Schemas in einer Schema-Registry), und Deployment, Observability und Fehlersuche werden komplexer. Zudem ist das Muster nicht konsequent umgesetzt: `catalog-api` und `reservation-worker` teilen dieselben Tabellen und denselben Datenbank-User, das Schema gehört den Migrationen des Workers. „Database per Service" ist damit verletzt, Schemaänderungen erfordern koordinierte Releases. Eine eigene, per Events aufgebaute Lesesicht der `catalog-api` würde das lösen, brächte aber Eventual Consistency in der Sitzplatzanzeige.

**Dynamic Scheduling** zeigt sich in Kubernetes durch die automatische Platzierung der Pods sowie durch die bedarfsgesteuerte Skalierung (Burns et al., 2016). In TicketFlow ist das vor allem für `reservation-worker` relevant: Das `ScaledObject` hält mindestens 2 Pods vor und erhöht deren Zahl bei wachsendem Consumer-Lag (Schwelle 10 Nachrichten je Pod, maximal 10 Pods bei 20 Partitionen). Topology-Spread-Constraints mit `DoNotSchedule` verteilen die Pods über die Regionen, damit ein einzelner Ausfall nicht die gesamte Verarbeitung stoppt. Einschränkend bietet die DHBWCloud nur eine echte Region. `region-a/b/c` sind simulierte Node-Labels. Das Scheduling-Verhalten lässt sich damit demonstrieren, echten Schutz vor einem Regionsausfall bietet es nicht.

Eine Alternative wäre eine feste Bereitstellung auf statischen VMs oder ein manuelles Hochsetzen der Replicas vor einem Event. Das wäre bei planbaren Verkaufsstarts praktikabel und einfacher zu verstehen, aber ressourcenintensiv (dauerhaft für die Spitze dimensioniert) oder fehleranfällig (falls jemand das Hochskalieren vergisst). Der Trade-off der gewählten Lösung: Zusätzliche Komponenten (KEDA, Lag-Abfrage) und Skalierungsgrenzen müssen konsistent abgestimmt werden, denn mehr Pods als Partitionen bringen nichts. Kritisch ist `DoNotSchedule` gerade im Fehlerfall: Fällt ein Worker-Node aus, zählt seine Region standardmäßig weiter als Domäne mit 0 Pods. Bei `maxSkew: 1` dürfen die übrigen Regionen dann nur je einen Pod tragen, weitere bleiben „Pending". KEDA kann während eines Ausfalls also faktisch nicht hochskalieren. `ScheduleAnyway` würde das vermeiden, aber die Verteilung nicht mehr garantieren. Dafür gewinnt man bessere Ressourcenausnutzung, Selbstheilung und eine Reaktion auf echte Nachfrage.


## Datenschutz und Datensicherheit

**Daten.** TicketFlow verarbeitet personenbezogene Daten in den Reservierungsdaten und Events: Vorname, Nachname, E-Mail-Adresse, gebuchten Sitzplatz sowie Session-ID. Das `ReservationRequested`-Event ist damit eindeutig einer Person zuordenbar. Auch `ReservationConfirmed` und `ReservationRejected` enthalten die E-Mail-Adresse (`customerEmail`), und `notification-worker` schreibt sie im Standardbetrieb ohne SMTP-Relay in sein Log. Neben der Datenbank sind also Kafka-Topics, Logs und gegebenenfalls Backups datenschutzrelevant. Zahlungsdaten werden nicht verarbeitet.

**Umgesetzte Maßnahmen.** HTTPS mit cert-manager-Zertifikaten am Ingress, TLS zur Datenbank (`sslmode=require`), getrennte Kafka-Identitäten (SCRAM) mit ACLs je Service, NetworkPolicies nach Deny-by-default in allen Namespaces, Secret-Management mit OpenBao und External Secrets (keine Zugangsdaten im Git), Schema-Validierung der Events sowie Basic-Auth vor AKHQ.

**DSGVO-Relevanz.** Die DSGVO ist klar relevant, da Name und E-Mail personenbezogene Daten nach Art. 4 Nr. 1 sind. Rechtsgrundlage wäre die Vertragserfüllung (Art. 6 Abs. 1 lit. b).
- *Zweckbindung:* Die Daten dürfen nur für Reservierung und Bestätigung verwendet werden, nicht für Marketing. Die Service-Trennung hilft, weil `notification-worker` auf Ergebnis-Events beschränkt ist. Die Informationspflichten nach Art. 13 erfüllt das Frontend mangels Datenschutzhinweis noch nicht.
- *Zugriffsschutz:* Kafka-ACLs und NetworkPolicies begrenzen den Zugriff technisch. Ein Rechtekonzept für Menschen (AKHQ, Datenbank) fehlt.
- *Speicherung und Löschung:* Kafka hält Events 7 Tage (`retention.ms`, `cleanup.policy: delete`). CNPG-Backups mit 7 Tagen Retention sind vorbereitet, aber deaktiviert (`backup.enabled: false`). Aktiviert müssten sie verschlüsselt und ins Löschkonzept einbezogen werden. Reservierungen in PostgreSQL werden nie gelöscht, für Löschanträge (Art. 17) gibt es keinen Prozess. Verteilte Kopien in Topics, Logs und Backups erschweren eine vollständige Löschung.

**Verbleibende Schwachstellen und offene Maßnahmen.**
1. Kafka läuft intern ohne TLS (`SASL_PLAINTEXT`). Personenbezogene Events sind im Cluster-Netz nur über NetworkPolicies abgeschirmt. Maßnahme: TLS-Listener.
2. Für Apicurio existiert ein anonymer Kafka-Listener (Port 9093), und `ANONYMOUS` ist Superuser. Einziger Schutz ist eine NetworkPolicy. Die ACLs je Service lassen sich darüber umgehen.
3. Unseal-Key und Root-Token von OpenBao liegen im Kubernetes-Secret `openbao-unseal-keys`, und k3s verschlüsselt Secrets in etcd nicht. Wer Secrets lesen kann, kann den Tresor öffnen. Maßnahme: Auto-Unseal über einen externen KMS, Root-Token widerrufen.
4. E-Mail-Adressen stehen im Klartext in Events und Logs. Maßnahme: Kunden-ID statt Name und E-Mail im Event, Maskierung im Log-Notifier.
5. Es gibt keine automatische Löschung von Reservierungsdaten und kein Rollenkonzept für AKHQ, wo Topic-Inhalte mit E-Mail-Adressen einsehbar sind.
6. Reservierungen erfordern keine Anmeldung und kein Rate Limiting. Bots können Flash Sales leerkaufen, und über fremde E-Mail-Adressen lassen sich Benachrichtigungen an Dritte auslösen.

Da personenbezogene Daten über Browser, API, Kafka, Worker, Datenbank und Backups wandern, braucht es ein gemeinsames Konzept für Lebenszyklus, Zugriff, Protokollierung und Löschung. Das macht Datensicherheit anspruchsvoller als im Monolithen.


## Literatur

- Burns, B., Grant, B., Oppenheimer, D., Brewer, E., & Wilkes, J. (2016). Borg, Omega, and Kubernetes. *Communications of the ACM, 59*(5), 50-57.

- KEDA Authors. (o. J.). *Apache Kafka Scaler* (KEDA Documentation 2.20). https://keda.sh/docs/2.20/scalers/apache-kafka/

- Kreps, J., Narkhede, N., & Rao, J. (2011). Kafka: A Distributed Messaging System for Log Processing. In *Proceedings of the NetDB Workshop 2011*.

- Patterns Library. (o. J.). https://www.cnpatterns.org/patterns-library

- PostgreSQL Global Development Group. (2026). *PostgreSQL 18 Documentation*. https://www.postgresql.org/docs/18/

- Verordnung (EU) 2016/679 (Datenschutz-Grundverordnung, DSGVO).
