# Laborbericht Teil 1 (Sturm)


## Architekturreflexion

TicketFlow ist eine Ticketverkaufsplattform für Live-Events. Der kritische Lastpfad entsteht bei Flash Sales, wenn sehr viele Nutzerinnen und Nutzer innerhalb weniger Sekunden eine begrenzte Zahl an Sitzplätzen reservieren wollen. Dafür nutzt TicketFlow eine cloud-native Architektur. `catalog-api` nimmt Reservierungsanfragen entgegen und schreibt nicht direkt in die Datenbank, sondern publiziert ein `ReservationRequested`-Event auf Kafka. `reservation-worker` verarbeitet die Anfrage asynchron, sperrt den Sitzplatz transaktional in PostgreSQL/CNPG und veröffentlicht anschließend `ReservationConfirmed` oder `ReservationRejected`. `notification-worker` verarbeitet die Ergebnis-Events unabhängig weiter.

Ein erster Vorteil ist die elastische Skalierung. Die eigentliche Arbeit liegt nicht in der HTTP-Annahme, sondern in der Verarbeitung und Konfliktbehandlung im Worker. Daher kann `reservation-worker` separat skaliert werden: KEDA skaliert ihn anhand des Kafka-Consumer-Lags von 2 bis 10 Pods. So bleibt bei einer Lastspitze die API arbeitsfähig, da die Verarbeitung schrittweise aufgeholt wird.

Ein zweiter Vorteil ist die Entkopplung. Kafka puffert Anfragen und trennt Frontend, API, Verarbeitung und Benachrichtigung voneinander. Fällt ein Worker temporär aus, gehen noch nicht verarbeitete Events nicht verloren. Das ist für TicketFlow relevant, weil ein Flash Sale nicht an einem kurzfristigen Ausfall der Verarbeitungsschicht scheitern soll.

Dem stehen jedoch klare Nachteile gegenüber. Erstens steigt die Betriebs- und Architekturkomplexität deutlich: Kubernetes, Kafka, PostgreSQL/CNPG, KEDA, ArgoCD und OpenBao müssen zusammenspielen. Allein das GitOps-Setup umfasst 21 ArgoCD-Applications, obwohl die fachliche Anwendung nur aus wenigen Services besteht. Zweitens wird die Logik schwerer nachvollziehbar, weil der Ablauf nicht mehr in einem einzigen synchronen Workflow endet, sondern über Events, Statuswerte und Wiederholungen verläuft. Der Nutzer erhält die Antwort „bestätigt" oder „abgelehnt" nicht mehr direkt, sondern erst nach dem Ergebnis-Event. Drittens bleibt die Datenbank der fachliche Engpass, weil jeder Sitzplatz nur einmal vergeben werden darf und die transaktionale Absicherung weiterhin in PostgreSQL erfolgen muss. Kafka beseitigt diesen Engpass nicht, sondern glättet nur die Schreiblast (Load Leveling): Die Datenbank wird in dem Tempo bedient, das sie verkraftet, statt von allen Anfragen gleichzeitig getroffen zu werden.

Eine realistische Alternative wäre ein Monolith auf einer einzelnen VM mit direktem Datenbank-Write. Das wäre einfacher zu entwickeln, leichter zu testen und deutlich weniger aufwendig im Betrieb. Für eine kleine, stabile Anwendung wäre das wahrscheinlich ausreichend. Bei TicketFlow würde ein solcher Ansatz bei Lastspitzen jedoch schnell an Grenzen stoßen, weil tausende gleichzeitige Requests direkt auf dieselben Sitzplatz-Zeilen in der Datenbank treffen. Die Folgen wären Timeouts und Lock-Konflikte, ohne Puffer dazwischen.

Eine zweite Alternative wäre eine klassische Drei-Schichten-Anwendung auf mehreren VMs hinter einem Load Balancer. Das verbessert die Skalierung der Web- und API-Schicht, bleibt aber bei der Reservierungsverarbeitung weiterhin stark synchron und zentral. Im Vergleich dazu ist die gewählte cloud-native Lösung besser auf den konkreten Lastfall von TicketFlow zugeschnitten, auch wenn sie technisch anspruchsvoller und betrieblich teurer ist. Unverhältnismäßig wird sie dort, wo die Last nicht spitzenförmig ist: Für einen kleinen Veranstalter mit wenigen hundert Tickets wären vier VMs, Kafka, ein Operator-Stack und 21 Applications überdimensioniert. Auch in unserer Umsetzung läuft die Plattform auf vier VMs der DHBWCloud. Die Ausfallsicherheit durch „Regionen" ist dort nur simuliert (siehe Dynamic Scheduling).

Die Entscheidung für Cloud Native ist damit vor allem vorteilhaft für den fachlichen Lastverlauf. TicketFlow profitiert davon, dass stateless Komponenten wie `catalog-api` und `notification-worker` leicht horizontal skaliert werden können, während die fachlich kritische Reservierungslogik isoliert im `reservation-worker` liegt. Gleichzeitig entsteht aber ein höherer Betriebsaufwand durch GitOps, Probes, Releases und die Abstimmung zwischen Anwendung, Datenbank und Event-Bus.


## Cloud-Native Patterns

Die folgenden zwei Muster orientieren sich an der [Patterns Library](https://www.cnpatterns.org/patterns-library): [Microservices Architecture](https://www.cnpatterns.org/development-design/microservices-architecture) (Development & Design) und [Dynamic Scheduling](https://www.cnpatterns.org/infrastructure-cloud/dynamic-scheduling) (Infrastructure & Cloud).

**Microservices Architecture** findet sich in der Aufteilung von TicketFlow wieder: `catalog-api`, `reservation-worker`, `notification-worker` und `frontend` sind getrennte, einzeln deploybare Komponenten mit klaren Aufgaben, die nur über HTTP bzw. Kafka-Events kommunizieren. Der `load-generator` ist dagegen reines Testwerkzeug und kein fachlicher Service. Das löst in der Domäne das Problem der Lastentkopplung: Die API kann Anfragen annehmen, während der Worker die eigentliche Reservierung verarbeitet, und nur der Worker muss beim Flash Sale stark skaliert werden. Die Benachrichtigung ist eine eigene Consumer-Group und kann ausfallen oder nachlaufen, ohne Reservierungen zu blockieren. Eine realistische Alternative wäre ein modularer Monolith mit derselben Fachlogik in einem Prozess. Das wäre einfacher zu betreiben, aber bei Flash Sales nur als Ganzes skalierbar, sodass auch der unkritische Teil mitskaliert würde. Der Trade-off besteht im höheren Verteilungsaufwand: Schnittstellen müssen versioniert werden (bei uns über JSON-Schemas in einer Schema-Registry), und Deployment, Observability und Fehlersuche werden komplexer. Dafür können die Teile unabhängig entwickelt, ausgerollt und skaliert werden.

**Dynamic Scheduling** zeigt sich in Kubernetes durch die automatische Platzierung der Pods sowie durch die bedarfsgesteuerte Skalierung. In TicketFlow ist das vor allem für `reservation-worker` relevant: Das `ScaledObject` hält mindestens 2 Pods vor und erhöht deren Zahl bei wachsendem Consumer-Lag (Schwelle 10 Nachrichten je Pod, maximal 10 Pods bei 20 Partitionen). Topology-Spread-Constraints mit `DoNotSchedule` verteilen die Pods über die Regionen, damit ein einzelner Ausfall nicht die gesamte Verarbeitung stoppt. Rechenressourcen werden so nur dann hochgefahren, wenn tatsächlich viele Reservierungen aufgelaufen sind. Einschränkend ist zu sagen, dass die DHBWCloud nur eine echte Region bietet; `region-a/b/c` sind simulierte Node-Labels. Das Scheduling-Verhalten lässt sich damit demonstrieren, echten Schutz vor einem Regionsausfall bietet es aber nicht.

Eine Alternative wäre eine feste Bereitstellung auf statischen VMs oder ein manuelles Hochsetzen der Replicas vor einem Event. Das wäre bei planbaren Verkaufsstarts durchaus praktikabel und einfacher zu verstehen, aber ressourcenintensiv (dauerhaft für die Spitze dimensioniert) oder fehleranfällig (falls jemand das Hochskalieren vergisst). Der Trade-off der gewählten Lösung: Zusätzliche Komponenten (KEDA, Metrik-Abfrage am Kafka-Lag) und Skalierungsgrenzen müssen konsistent abgestimmt werden. Mehr Pods als Partitionen bringen nichts, und `DoNotSchedule` lässt Pods im Zustand „Pending", wenn eine Region keine Kapazität hat. Dafür gewinnt man bessere Ressourcenausnutzung, Selbstheilung und eine passende Reaktion auf echte Nachfrage. Ergänzend sorgen Readiness-/Liveness-Probes, Rolling Updates und ein PodDisruptionBudget dafür, dass fehlerhafte Pods keinen Traffic erhalten und Updates den Betrieb nicht unterbrechen.


## Datenschutz und Datensicherheit

**Daten.** TicketFlow verarbeitet personenbezogene Daten direkt in den Reservierungsdaten und in den Events: Vorname, Nachname, E-Mail-Adresse und gebuchten Sitzplatz sowie Session-ID. Im `ReservationRequested`-Event stehen Name, E-Mail und Sitzplatz, damit ist die Nachricht eindeutig einer Person zuordenbar. Auch `ReservationConfirmed` und `ReservationRejected` enthalten die E-Mail-Adresse direkt (`customerEmail`), und `notification-worker` schreibt sie im Standardbetrieb ohne SMTP-Relay in sein Log. Damit sind nicht nur die Datenbank, sondern auch Kafka-Topics und Logs (und bei aktivierten Backups auch diese) datenschutzrelevant. Zahlungsdaten werden nicht verarbeitet.

**Umgesetzte Maßnahmen.** Die Umsetzung hat gute Grundlagen: getrennte Kafka-Identitäten (SCRAM) mit ACLs je Service, NetworkPolicies nach Deny-by-default in allen Namespaces, zentrales Secret-Management mit OpenBao und External Secrets (keine Zugangsdaten im Git), Schema-Validierung der Events sowie Basic-Auth vor AKHQ.

**Ergänzungen vor produktivem Einsatz.**
- Datenminimierung: eine Kunden-ID statt Name und E-Mail im Event, sensible Daten ganz vom Event-Bus trennen.
- Keine personenbezogenen Daten in Logs (`customerEmail` im Log-Notifier entfernen oder maskieren).
- TLS für Kafka und verschlüsselte Backups.
- Ein Rollen- und Rechtekonzept für AKHQ, da sich dort Topic-Inhalte mit E-Mail-Adressen einsehen lassen.
- Ein definierter Löschprozess (siehe unten).

**DSGVO-Relevanz.** Die DSGVO ist klar relevant, da Name und E-Mail personenbezogene Daten nach Art. 4 Nr. 1 sind. Rechtsgrundlage wäre die Vertragserfüllung (Art. 6 Abs. 1 lit. b), also die Abwicklung der Reservierung.
- *Zweckbindung:* Die Daten dürfen nur für Reservierung und Bestätigung verwendet werden. Eine Nutzung für Marketing wäre davon nicht gedeckt. Die Trennung in Services hilft hier, weil der `notification-worker` auf Ergebnis-Events beschränkt ist.
- *Zugriffsschutz:* Kafka-ACLs und NetworkPolicies begrenzen den Zugriff technisch. Ein Rechtekonzept für Menschen (AKHQ, Datenbank) fehlt jedoch.
- *Speicherung und Löschung:* In Kafka werden Events 7 Tage gehalten (`retention.ms`, `cleanup.policy: delete`). Das CNPG-Chart kann Backups mit 7 Tagen Retention rendern, ist aber standardmäßig deaktiviert (`backup.enabled: false`), sodass aktuell keine Backup-Kopien entstehen. Würden sie aktiviert, müssten sie verschlüsselt und in die Löschkonzeption einbezogen werden. Reservierungen in PostgreSQL werden dagegen nie gelöscht, und für Löschanträge (Art. 17) gibt es keinen Prozess. Verteilte Kopien in Topics, Logs und Backups erschweren eine vollständige Löschung. Das Löschen in der Datenbank allein genügt nicht.

**Verbleibende Schwachstellen.**
1. Kafka läuft intern ohne TLS (`SASL_PLAINTEXT`). Personenbezogene Events sind im Cluster-Netz daher unverschlüsselt und nur über NetworkPolicies abgeschirmt.
2. Die E-Mail-Adresse steht im Klartext im Log des `notification-worker` und in den Events.
3. Es gibt keine automatische Löschung von Reservierungsdaten.

Datenschutz in einer verteilten Architektur ist nicht mit einer einzelnen Maßnahme abgeschlossen: Personenbezogene Daten wandern über Browser, API, Kafka, Worker, Datenbank und Backups. Das macht ein gemeinsames Schutzkonzept für Lebenszyklus, Zugriff, Protokollierung und Löschung nötig und die Datensicherheit anspruchsvoller als in einem Monolithen. Beherrschbar ist es, wenn diese Prozesse von Anfang an mitgedacht werden.


## Literatur

- Apache. (o. J.). *KEDA | Apache Kafka*. https://keda.sh/docs/2.20/scalers/apache-kafka/

- Burns, B., Grant, B., Oppenheimer, D., Brewer, E., & Wilkes, J. (2016). *Borg, Omega, and Kubernetes*. Communications of the ACM, 59(5), 50-57.

- Kreps, J. (2011). *Kafka: A Distributed Messaging System for Log Processing*.

- Patterns Library. (o. J.). https://www.cnpatterns.org/patterns-library

- PostgreSQL. (2026). *PostgreSQL 18 Documentation*. https://www.postgresql.org/docs/18/

- Regulation (EU) 2016/679. *Datenschutz-Grundverordnung (DSGVO)*.
