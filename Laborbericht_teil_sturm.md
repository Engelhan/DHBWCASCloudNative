# Laborbericht Teil 1 (Sturm)

## Architekturreflexion

TicketFlow ist eine Ticketverkaufsplattform fuer Live-Events. Der kritische Lastpfad entsteht bei Flash Sales, wenn sehr viele Nutzerinnen und Nutzer innerhalb weniger Sekunden eine begrenzte Zahl an Sitzplaetzen reservieren wollen. Dafuer nutzt TicketFlow eine cloud-native Architektur. `catalog-api` nimmt Reservierungsanfragen entgegen und schreibt nicht direkt in die Datenbank, sondern publiziert ein `ReservationRequested`-Event auf Kafka. `reservation-worker` verarbeitet die Anfrage asynchron, sperrt den Sitzplatz transaktional in PostgreSQL/CNPG und veroeffentlicht anschliessend `ReservationConfirmed` oder `ReservationRejected`. `notification-worker` verarbeitet die Ergebnis-Events unabhaengig weiter.

Ein erster Vorteil ist die elastische Skalierung. Die eigentliche Arbeit liegt nicht in der HTTP-Annahme, sondern in der Verarbeitung und Konfliktbehandlung im Worker. Daher kann `reservation-worker` separat skaliert werden, etwa ueber KEDA anhand des Kafka-Consumer-Lags. Bei einer Lastspitze bleibt die API reaktionsfaehig, waehrend die Verarbeitung schrittweise aufgeholt wird.

Ein zweiter Vorteil ist die Entkopplung. Kafka puffert Anfragen und trennt Frontend, API, Verarbeitung und Benachrichtigung voneinander. Faellt ein Worker temporaer aus, gehen noch nicht verarbeitete Events nicht sofort verloren. Das ist fuer TicketFlow relevant, weil ein Flash Sale nicht an einem kurzfristigen Ausfall der Verarbeitungsschicht scheitern soll. 

Dem stehen jedoch klare Nachteile gegenueber. Erstens steigt die Betriebs- und Architekturkomplexitaet deutlich: Kubernetes, Kafka, PostgreSQL/CNPG, KEDA, ArgoCD und Monitoring muessen zusammenspielen. Zweitens wird die Logik schwerer nachvollziehbar, weil der Ablauf nicht mehr in einem einzigen synchronen Request endet, sondern ueber Events, Statuswerte und Wiederholungen verlaeuft. Hinzu kommt, dass die Datenbank der fachliche Engpass bleibt, weil jeder Sitzplatz nur einmal vergeben werden darf und die transaktionale Absicherung weiterhin in PostgreSQL erfolgen muss.

Eine realistische Alternative waere ein Monolith auf einer einzelnen VM mit direktem Datenbank-Write. Das waere einfacher zu entwickeln, leichter zu testen und deutlich weniger aufwendig im Betrieb. Fuer eine kleine, stabile Anwendung waere das wahrscheinlich ausreichend. Bei TicketFlow wuerde ein solcher Ansatz bei Lastspitzen jedoch schnell an Grenzen stossen, weil die Datenbank dann zum zentralen Flaschenhals wird.

Eine zweite Alternative waere eine klassische Drei-Schichten-Anwendung auf mehreren VMs hinter einem Load Balancer. Das verbessert die Skalierung der Web- und API-Schicht, bleibt aber bei der Reservierungsverarbeitung weiterhin stark synchron und zentral. Im Vergleich dazu ist die gewaehlte cloud-native Loesung besser auf den konkreten Lastfall von TicketFlow zugeschnitten, auch wenn sie technisch anspruchsvoller und betrieblich teurer ist. Fuer sehr kleine oder selten genutzte Ticketprojekte waere sie eher ueberdimensioniert.

Die Entscheidung fuer Cloud Native ist damit vor allem eine Reaktion auf den fachlichen Lastverlauf und nicht auf einen abstrakten Technologietrend. TicketFlow profitiert davon, dass stateless Komponenten wie `catalog-api` und `notification-worker` leicht horizontal skaliert werden koennen, waehrend die fachlich kritische Reservierungslogik isoliert im `reservation-worker` liegt. Gleichzeitig entsteht aber ein hoeherer Betriebsaufwand durch GitOps, Probes, Releases und die Abstimmung zwischen Anwendung, Datenbank und Event-Bus. Genau an dieser Stelle zeigt sich der eigentliche Trade-off: bessere Elastizitaet und Robustheit gegen Lastspitzen gegen mehr organisatorische und technische Komplexitaet.

 
## Cloud-Native Patterns

Die folgenden zwei Muster orientieren sich an der [Patterns Library](https://www.cnpatterns.org/patterns-library): [Microservices Architecture](https://www.cnpatterns.org/development-design/microservices-architecture) und [Dynamic Scheduling](https://www.cnpatterns.org/infrastructure-cloud/dynamic-scheduling).

**Microservices Architecture** findet sich in der Aufteilung von TicketFlow wieder: `catalog-api`, `reservation-worker`, `notification-worker`, `frontend` und `load-generator` sind getrennte Komponenten mit klaren Aufgaben. Das loest in der Domäne vor allem das Problem der Last- und Entwicklungsentkopplung. Die API kann Anfragen annehmen, waehrend der Worker die eigentliche Reservierung verarbeitet; Benachrichtigung und Lasttest bleiben ebenfalls getrennt. Eine realistische Alternative waere ein Monolith mit derselben Fachlogik in einer Anwendung. Das waere einfacher zu betreiben, aber bei Flash Sales schwerer zu skalieren und bei aendernden Anforderungen unflexibler. Der Trade-off besteht im hoeheren Verteilungsaufwand: Schnittstellen, Observability, Deployment und Fehlersuche werden komplexer, dafuer koennen die Teile unabhaengig entwickelt und skaliert werden.

**Dynamic Scheduling** zeigt sich in Kubernetes durch die Platzierung und Verteilung der Pods sowie durch horizontale Skalierung. In TicketFlow ist das vor allem fuer `reservation-worker` relevant, weil mehrere Replikas, Pod-Anti-Affinity und KEDA die Lastspitzen aus dem Flash Sale besser abfangen. Das Problem in der Domäne ist eindeutig: Ein einzelner Worker darf nicht zum Single Point of Failure werden, und Rechenressourcen sollen nur dann hochgefahren werden, wenn wirklich viele Reservierungen aufgelaufen sind. Eine Alternative waere eine feste Bereitstellung auf statischen VMs oder ein manuelles Hochsetzen der Replicas bei Bedarf. Der Trade-off besteht in hoeherer Infrastruktur-Komplexitaet und mehr Abstimmungsaufwand bei Skalierungsgrenzen, dafuer aber in besserer Ausnutzung, Selbstheilung und einer deutlich passenderen Reaktion auf echte Nachfrage.

Ein weiterer praktischer Vorteil ist, dass Kubernetes die fuer TicketFlow relevanten Betriebsmechanismen gleich mitliefert: Readiness- und Liveness-Probes sorgen dafuer, dass fehlerhafte Pods nicht am Traffic teilnehmen, und Rolling Updates reduzieren Ausfallzeiten bei Deployments. Fuer die Anwendung ist das besonders wichtig, weil `catalog-api` und `reservation-worker` ohne solche Mechanismen zwar lauffaehig, aber unter Last und bei Updates deutlich fragiler waeren. Dynamic Scheduling ist hier also nicht nur ein Orchestrierungsdetail, sondern eine Voraussetzung dafuer, dass die Anwendung auch bei Störungen und Replikationsaenderungen kontrolliert weiterlaeuft.

 
## Datenschutz und Datensicherheit

TicketFlow verarbeitet personenbezogene Daten direkt in den Reservierungsdaten und in den Events. Im `ReservationRequested`-Event stehen Name, E-Mail, Session-ID, Event-ID und Sitzplatz; damit ist die Nachricht eindeutig einer Person zuordenbar. Auch `ReservationConfirmed` und `ReservationRejected` enthalten solche Bezuge zumindest indirekt ueber Reservierungs- und Kundendaten. Damit sind nicht nur die Datenbank, sondern auch Kafka-Topics, Logs und Backups datenschutzrelevant.

Technisch werden Konfiguration und geheime Werte getrennt. Nicht vertrauliche Parameter laufen ueber ConfigMaps, Zugangsdaten ueber Kubernetes-Secrets. Die eigentliche Reservierung wird transaktional im Worker abgesichert, sodass ein Sitzplatz nicht doppelt vergeben werden kann. Fuer einen produktiven Betrieb waeren zusaetzlich RBAC mit Least Privilege, NetworkPolicies, verschluesselte Secret-Verwaltung, regelmaessige Backups und Restore-Tests erforderlich.

Die DSGVO ist fuer TicketFlow klar relevant, sobald echte Nutzerinnen und Nutzer verarbeitet werden. Dann gelten Zweckbindung, Datenminimierung, Speicherbegrenzung und Loeschkonzepte. TicketFlow braucht die Daten nur fuer Reservierung, Statusanzeige und gegebenenfalls Benachrichtigung; alles darueber hinaus waere zu begruenden. Eine verbleibende Schwachstelle ist die Durchsetzung von Loesch- und Aufbewahrungsregeln ueber alle beteiligten Systeme hinweg. Im aktuellen Stand ist die funktionale Verarbeitung korrekt, aber fuer einen produktiven Einsatz waeren klare Retention-Regeln fuer Topics, Logs und Backups weiterhin sinnvoll.

Hinzu kommt, dass Datenschutz in einer verteilten Architektur nicht mit einer einzelnen Massnahme abgeschlossen ist. Selbst wenn Secrets sauber getrennt sind, koennen personenbezogene Daten ueber mehrere Systeme wandern: Browser, API, Kafka, Worker, Datenbank, Observability-Stack und Backups. Daraus folgt fuer TicketFlow ein gemeinsames Schutzkonzept fuer Lebenszyklus, Zugriff, Protokollierung und Loeschung. Gerade diese Verteilung macht die Datensicherheit anspruchsvoller als in einem Monolithen, aber sie ist beherrschbar, wenn die Prozesse von Anfang an mitgedacht werden.


## Literatur

- Apache. (o. J.). *KEDA | Apache Kafka*. https://keda.sh/docs/2.20/scalers/apache-kafka/

- Burns, B., Grant, B., Oppenheimer, D., Brewer, E., & Wilkes, J. (2016). *Borg, Omega, and Kubernetes*. Communications of the ACM, 59(5), 50-57.

- Kreps, J. (2011). *Kafka: A Distributed Messaging System for Log Processing*.

- Patterns Library. (o. J.). https://www.cnpatterns.org/patterns-library

- PostgreSQL. (2026). *PostgreSQL 18 Documentation*. https://www.postgresql.org/docs/18/

- Regulation (EU) 2016/679. *Datenschutz-Grundverordnung (DSGVO)*.