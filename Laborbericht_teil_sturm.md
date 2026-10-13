# Laborbericht Teil 1 (Sturm)


## Architekturreflexion

TicketFlow ist eine Ticketverkaufsplattform für Live-Events. Der kritische Lastpfad entsteht bei Flash Sales, wenn sehr viele Nutzerinnen und Nutzer innerhalb weniger Sekunden eine begrenzte Zahl an Sitzplätzen reservieren wollen. Dafür nutzt TicketFlow eine cloud-native Architektur. `catalog-api` nimmt Reservierungsanfragen entgegen und schreibt nicht direkt in die Datenbank, sondern publiziert ein `ReservationRequested`-Event auf Kafka. `reservation-worker` verarbeitet die Anfrage asynchron, sperrt den Sitzplatz transaktional in PostgreSQL/CNPG und veröffentlicht anschliessend `ReservationConfirmed` oder `ReservationRejected`. `notification-worker` verarbeitet die Ergebnis-Events unabhängig weiter.

Ein erster Vorteil ist die elastische Skalierung. Die eigentliche Arbeit liegt nicht in der HTTP-Annahme, sondern in der Verarbeitung und Konfliktbehandlung im Worker. Daher kann `reservation-worker` separat skaliert werden, z.B. über KEDA anhand des Kafka-Consumer-Lags. So bleibt bei einer Lastspitze die API arbeitsfähig, da die Verarbeitung schrittweise aufgeholt wird.

Ein zweiter Vorteil ist die Entkopplung. Kafka puffert Anfragen und trennt Frontend, API, Verarbeitung und Benachrichtigung voneinander. Fällt ein Worker temporär aus, gehen noch nicht verarbeitete Events nicht verloren. Das ist für TicketFlow relevant, weil ein Flash Sale nicht an einem kurzfristigen Ausfall der Verarbeitungsschicht scheitern soll.

Dem stehen jedoch klare Nachteile gegenüber. Erstens steigt die Betriebs- und Architekturkomplexität deutlich. So müssen Kubernetes, Kafka, PostgreSQL/CNPG, KEDA, ArgoCD und Monitoring zusammenspielen. Zweitens wird die Logik schwerer nachvollziehbar, weil der Ablauf nicht mehr in einem einzigen synchronen Workflow endet, sondern über Events, Statuswerte und Wiederholungen verläuft. Hinzu kommt, dass die Datenbank der fachliche Engpass bleibt, weil jeder Sitzplatz nur einmal vergeben werden darf und die transaktionale Absicherung weiterhin in PostgreSQL erfolgen muss.

Eine realistische Alternative wäre ein Monolith auf einer einzelnen VM mit direktem Datenbank-Write. Das wäre einfacher zu entwickeln, leichter zu testen und deutlich weniger aufwendig im Betrieb. Für eine kleine, stabile Anwendung wäre das wahrscheinlich ausreichend. Bei TicketFlow würde ein solcher Ansatz bei Lastspitzen jedoch schnell an Grenzen stoßen, weil die Datenbank dann zum zentralen Flaschenhals wird.

Eine zweite Alternative wäre eine klassische Drei-Schichten-Anwendung auf mehreren VMs hinter einem Load Balancer. Das verbessert die Skalierung der Web- und API-Schicht, bleibt aber bei der Reservierungsverarbeitung weiterhin stark synchron und zentral. Im Vergleich dazu ist die gewählte cloud-native Lösung besser auf den konkreten Lastfall von TicketFlow zugeschnitten, auch wenn sie technisch anspruchsvoller und betrieblich teurer ist. Für sehr kleine oder selten genutzte Ticketprojekte wäre sie eher überdimensioniert.

Die Entscheidung für Cloud Native ist damit vor allem vorteilhaft für den fachlichen Lastverlauf. TicketFlow profitiert davon, dass stateless Komponenten wie `catalog-api` und `notification-worker` leicht horizontal skaliert werden können, während die fachlich kritische Reservierungslogik isoliert im `reservation-worker` liegt. Gleichzeitig entsteht aber ein höherer Betriebsaufwand durch GitOps, Probes, Releases und die Abstimmung zwischen Anwendung, Datenbank und Event-Bus.


## Cloud-Native Patterns

Die folgenden zwei Muster orientieren sich an der [Patterns Library](https://www.cnpatterns.org/patterns-library): [Microservices Architecture](https://www.cnpatterns.org/development-design/microservices-architecture) und [Dynamic Scheduling](https://www.cnpatterns.org/infrastructure-cloud/dynamic-scheduling).

**Microservices Architecture** findet sich in der Aufteilung von TicketFlow wieder: `catalog-api`, `reservation-worker`, `notification-worker`, `frontend` und `load-generator` sind getrennte Komponenten mit klaren Aufgaben. Das löst in der Domäne vor allem das Problem der Last- und Entwicklungsentkopplung. Die API kann Anfragen annehmen, während der Worker die eigentliche Reservierung verarbeitet. Benachrichtigung und Lasttest bleiben ebenfalls getrennt. Eine realistische Alternative wäre ein Monolith mit derselben Fachlogik in einer Anwendung. Das wäre einfacher zu betreiben, aber bei Flash Sales schwerer zu skalieren und bei ändernden Anforderungen unflexibler. Der Trade-off besteht im höheren Verteilungsaufwand: Schnittstellen, Observability, Deployment und Fehlersuche werden komplexer, dafür können die Teile unabhängig entwickelt und skaliert werden.

**Dynamic Scheduling** zeigt sich in Kubernetes durch die Platzierung und Verteilung der Pods sowie durch horizontale Skalierung. In TicketFlow ist das vor allem für `reservation-worker` relevant, weil mehrere Replikas, Pod-Anti-Affinity und KEDA die Lastspitzen aus dem Flash Sale besser abfangen. Ein einzelner Worker darf nicht zum Single Point of Failure werden, und Rechenressourcen sollen nur dann hochgefahren werden, wenn wirklich viele Reservierungen aufgelaufen sind. Eine Alternative wäre eine feste Bereitstellung auf statischen VMs oder ein manuelles Hochsetzen der Replicas bei Bedarf. Der Trade-off besteht in höherer Infrastruktur-Komplexität und mehr Abstimmungsaufwand bei Skalierungsgrenzen, dafür aber in besserer Ausnutzung, Selbstheilung und einer deutlich passenderen Reaktion auf echte Nachfrage.

Ein weiterer Vorteil ist, dass Kubernetes die für TicketFlow relevanten Betriebsmechanismen mitliefert. Readiness- und Liveness-Probes sorgen dafür, dass fehlerhafte Pods nicht am Traffic teilnehmen, und Rolling Updates reduzieren Ausfallzeiten bei Deployments. Für die Anwendung ist das besonders wichtig, weil `catalog-api` und `reservation-worker` ohne solche Mechanismen zwar lauffähig, aber unter Last und bei Updates deutlich fragiler wären. Dynamic Scheduling ist hier also nicht nur ein Orchestrierungsdetail, sondern eine Voraussetzung dafür, dass die Anwendung auch bei Störungen und Replikationsänderungen kontrolliert weiterläuft.


## Datenschutz und Datensicherheit

TicketFlow verarbeitet personenbezogene Daten direkt in den Reservierungsdaten und in den Events. Daher ist die DSGVO ist für TicketFlow klar relevant. Im `ReservationRequested`-Event stehen Name, E-Mail und Sitzplatz. Damit ist die Nachricht eindeutig einer Person zuordenbar. Auch `ReservationConfirmed` und `ReservationRejected` enthalten solche Bezüge zumindest indirekt über Reservierungs- und Kundendaten. Damit sind nicht nur die Datenbank, sondern auch Kafka-Topics, Logs und Backups datenschutzrelevant.

Die aktuelle Umsetzung hat gute Grundlagen, wie getrennte Secrets, NetworkPolicies und ein zentrales Secret-Management, aber sie ist aus Datenschutzsicht noch nicht ausreichend. Für einen produktiven Betrieb müsste man deutlich stärker auf Datenminimierung setzen. D.h. weniger personenbezogene Daten im Event-Bus, keine PII in Logs, klare Retention- und Löschfristen, verschlüsselte Backups und eine technische Durchsetzung von Zugriffsbeschränkungen. Gerade im Event-Driven Design ist es wichtig, nur die minimal nötigen Daten zu transportieren und sensible Informationen so früh wie möglich zu pseudonymisieren, z.B. über eine user id oder zu trennen.

Hinzu kommt, dass Datenschutz in einer verteilten Architektur nicht mit einer einzelnen Maßnahme abgeschlossen ist. Es können personenbezogene Daten über mehrere Systeme wandern: Browser, API, Kafka, Worker, Datenbank, Observability-Stack (falls vorhanden, in diesem Fall nicht) und Backups. Daraus folgt für TicketFlow ein gemeinsames Schutzkonzept für Lebenszyklus, Zugriff, Protokollierung und Löschung. Gerade diese Verteilung macht die Datensicherheit anspruchsvoller als in einem Monolithen, aber sie ist beherrschbar, insofern die Prozesse von Anfang an mitgedacht werden.


## Literatur

- Apache. (o. J.). *KEDA | Apache Kafka*. https://keda.sh/docs/2.20/scalers/apache-kafka/

- Burns, B., Grant, B., Oppenheimer, D., Brewer, E., & Wilkes, J. (2016). *Borg, Omega, and Kubernetes*. Communications of the ACM, 59(5), 50-57.

- Kreps, J. (2011). *Kafka: A Distributed Messaging System for Log Processing*.

- Patterns Library. (o. J.). https://www.cnpatterns.org/patterns-library

- PostgreSQL. (2026). *PostgreSQL 18 Documentation*. https://www.postgresql.org/docs/18/

- Regulation (EU) 2016/679. *Datenschutz-Grundverordnung (DSGVO)*.