# charts/ – Helm-Packaging (W1)

## Muster für Applikations-Charts (verbindlich)

Alle Charts, die eigenen Anwendungscode ausrollen — [`ticketflow-app/`](ticketflow-app/)
(catalog-api + frontend), [`reservation-worker/`](reservation-worker/),
[`notification-worker/`](notification-worker/) — folgen demselben Aufbau. Referenz ist
`reservation-worker`; Abweichungen sind zu begründen, nicht stillschweigend einzuführen.

```
charts/<name>/
├── Chart.yaml               # name == Verzeichnisname
├── README.md
├── values.yaml              # feste Key-Reihenfolge, siehe unten
├── values-dev.yaml          # kleine Werte fuer die Lab-VMs
├── values-prod.yaml         # HA-Werte
├── values.schema.json       # Pflichtfelder + Typen, verbietet image.tag "latest"
└── templates/
    ├── _helpers.tpl         # name / fullname / chart / labels / selectorLabels / image
    ├── deployment.yaml
    ├── configmap.yaml
    ├── secret.yaml                    # nur wo der Chart selbst Credentials anlegt
    ├── service.yaml                   # nur wo es HTTP-Konsumenten gibt
    ├── servicemonitor.yaml            # mit Service; sonst podmonitor.yaml
    ├── networkpolicy.yaml             # default aus
    └── poddisruptionbudget.yaml
```

Feste Key-Reihenfolge in `values.yaml` (je Komponente, falls der Chart mehrere rendert):
`image`, `replicaCount`, `resources`, `podSecurityContext`, `securityContext`, `config`,
`database`/`databaseSecret`/`secret`, `service`, `ingress`, `serviceMonitor`/`podMonitor`,
`networkPolicy`, `podDisruptionBudget`, `topologySpreadConstraint`.

Regeln:

- **Namen und Labels immer über `_helpers.tpl`**, nie wörtlich im Template. Jede Ressource trägt
  das volle `app.kubernetes.io/`-Set; Charts mit mehreren Komponenten zusätzlich
  `app.kubernetes.io/component`.
- **Kein `namespace:` in Templates und keines in `values.yaml`** — den Namespace bestimmt die
  ArgoCD-Application (`destination.namespace` in `gitops/applicationset.yaml`).
- **`securityContext` kommt aus `values.yaml`**, nie fest aus dem Template.
- **Probes `/healthz` (liveness) und `/readyz` (readiness)**; `/metrics` wird gescrapt.
- **Kein `:latest`** — Registry und Tag kommen zentral (siehe nächster Abschnitt), das Schema
  verbietet `latest` zusätzlich.
- `_helpers.tpl` ist je Chart dupliziert statt als Library-Chart ausgelagert: für drei kurze
  Defines wäre eine `file://`-Dependency pro Chart mehr Aufwand als Nutzen.

`apicurio` und `akhq` folgen diesem Muster bewusst nur teilweise — sie sind Plattform-Werkzeuge
(Schema-Registry, Kafka-UI), kein TicketFlow-Anwendungscode; `postgres-cluster` und
`kafka-cluster` rendern ausschließlich Custom Resources fremder Operatoren und haben daher weder
Deployment noch eigene Labels.

## Namespace-Aufteilung

Die Kafka-Plattform läuft getrennt von der Anwendung:

| Namespace | Charts |
|---|---|
| `ticketflow` | `ticketflow-app` (catalog-api + frontend), `reservation-worker`, `notification-worker` |
| `kafka` | `strimzi-operator`, `kafka-cluster`, `apicurio`, `akhq` |
| `application` | `postgres-cluster` |
| `monitoring`, `longhorn-system`, `cert-manager`, `external-dns`, `cnpg-system`, `keda` | die jeweiligen Addons |

Beide App-Namen stehen in `gitops/env/cluster.yaml` (`ticketflow.namespaces.app`/`.kafka`); jeder
Chart, der den Kafka-Bootstrap-Service oder die Apicurio-Registry braucht, leitet die Adresse
daraus ab (`<clusterName>-kafka-bootstrap.<kafkaNs>.svc:9092` bzw.
`http://apicurio.<kafkaNs>.svc:8080/...`) statt sie auszuschreiben — vorher stand derselbe
DNS-Name in vier `values.yaml` parallel.

**Eine Ausnahme, die man kennen muss:** `charts/kafka-cluster` rendert seine `KafkaUser`-CRs
nicht in den Kafka-, sondern in den App-Namespace. Der Strimzi User Operator legt das generierte
SCRAM-`Secret` immer im Namespace der CR ab, und `secretKeyRef` überschreitet keine
Namespace-Grenze — die Services kämen sonst nicht an ihr Passwort. Dafür setzt der Chart
`spec.entityOperator.userOperator.watchedNamespace`; siehe `kafka-cluster/README.md`
## Namespaces.

`charts/akhq` ist der einzige Client, der den Bootstrap-Service nicht ableitet, sondern als
kurzen In-Namespace-Namen ausschreibt: sein Wert liegt in den Values des gewrappten
Upstream-Charts, und AKHQ läuft ohnehin im selben Namespace wie der Cluster.

## Zentrale Werte: `gitops/env/cluster.yaml`

Image-Registry, Image-Tag, DNS-Zone, ACME-Kontakt und TSIG-Key-Name stehen in **keinem** Chart,
sondern einmalig in [`../gitops/env/cluster.yaml`](../gitops/env/cluster.yaml). ArgoCD reicht die
Datei jedem Chart per Multi-Source `$values` als zusätzliche Values-Datei durch (siehe
`../gitops/applicationset.yaml`); die Charts lesen sie als `.Values.ticketflow.*`.

Jeder betroffene Chart deklariert dafür ein leeres `ticketflow: {}` in seiner `values.yaml` —
nur damit ein Rendern ohne die zentrale Datei eine verständliche `required`-Meldung liefert statt
eines Nil-Pointers. **Lokal muss sie deshalb immer mitgegeben werden:**

```bash
helm template <name> charts/<name> \
  -f charts/<name>/values.yaml \
  -f gitops/env/cluster.yaml
```

Zwei Wrapper-Charts brauchen den Wert *innerhalb* der Values ihres Upstream-Sub-Charts, wo Helm
nicht templatet — gelöst ohne index-basierte `helm.parameters`:

- **`akhq`**: `akhq.ingress.enabled: false`, stattdessen ein chart-eigenes
  `templates/ingress.yaml`, das den Host aus `ticketflow.dnsZone` bildet.
- **`external-dns`**: `templates/zone-configmap.yaml` schreibt Zone/Server/Key-Name in eine
  ConfigMap; die Upstream-Values referenzieren sie als `$(DNS_ZONE)` usw. Kubernetes expandiert
  `$(VAR)` in `args` — derselbe Mechanismus, den der Chart für das TSIG-Secret schon nutzt.

Welche `values-<env>.yaml` greift, steuert `ticketflow.environment` in derselben Datei.

## `ticketflow-app` — eigenständiger Chart für `catalog-api` + `frontend`

[`ticketflow-app/`](ticketflow-app/) bündelt `catalog-api` und `frontend` (Philipps Scope) in
einem gemeinsamen Chart — anders als `notification-worker`/`reservation-worker` (je ein Service
pro Chart), weil `frontend` ausschließlich mit `catalog-api` spricht und sich mit ihm eine
Ingress-Route/Origin teilt. Team-Entscheidung: bewusst nicht Teil von `charts/kafka-cluster`
(Kafka) — zwei getrennte ArgoCD-`Application`s (die Elemente `kafka-cluster` und `ticketflow-app`
in `../gitops/applicationset.yaml`), damit ein Sync/Rollback des einen den
anderen nicht anfasst. Beide laufen im selben Kubernetes-Namespace (`ticketflow`), damit
`catalog-api` den Kafka-Bootstrap-Service per kurzem In-Namespace-DNS-Namen erreicht.

## `notification-worker` — eigenständiger Chart statt Teil des Umbrella-Charts

[`notification-worker/`](notification-worker/) ist der erste vollständig implementierte Service
im Repo und bekam deshalb, abweichend vom ursprünglichen Plan (alle drei Services im
Umbrella-Chart), einen eigenen Chart nach Pattern (b) unten (volleigene Templates, keine
Upstream-`dependencies`) — analog zu [`cert-manager-config/`](cert-manager-config/)/
[`argocd-ingress/`](argocd-ingress/), aber mit `_helpers.tpl`, da er mit fünf Ressourcen
(Deployment, ConfigMap, Secret, PodDisruptionBudget, PodMonitor, optional NetworkPolicy)
deutlich mehr rendert als die anderen Custom-Charts. Bewusst kein `Service` — es gibt keinen
anderen Konsumenten der HTTP-Endpunkte (reiner Kafka-Consumer, Kubelet-Probes gehen direkt an den
Pod), daher scraped ein `PodMonitor` (statt `ServiceMonitor`) `/metrics` direkt über den
Pod-Label-Selector, siehe `notification-worker/templates/podmonitor.yaml`. Default sind `replicaCount: 3` + `PodDisruptionBudget`. Seit der Umstellung auf das ApplicationSet ist er als Element
`notification-worker` mit ausgerollt — vorher fehlte ihm eine Application, und `image.tag` war
leer, sodass ein Deploy ohne `--set` gar nicht möglich war. 

## `reservation-worker` — eigenständiger Chart, gleiches Muster wie `notification-worker`

[`reservation-worker/`](reservation-worker/) ist der zweite vollständig implementierte Service,
analog zu `notification-worker` als eigener Chart (Deployment, ConfigMap, Secret,
PodDisruptionBudget, PodMonitor, optionale NetworkPolicy, `_helpers.tpl`, eigenes
`values.schema.json`) — deployt als Element `reservation-worker`. Abweichungen von
`notification-worker`s Vorlage: `replicaCount: 1` statt `3` — der Service wird von KEDA auf
Kafka-Consumer-Lag skaliert (P6, Element `keda`), ein hoher statischer Default wäre daneben
irreführend; und ein zusätzliches `database`/`databaseSecret`-Wertepaar statt
`notification-worker`s SMTP/TLS-Secret-Feldern, weil der Service transaktional in CNPG-Postgres
schreibt (siehe `src/reservation-worker/README.md` ## Zweck).

**Postgres-Anbindung (gelöst, 2026-09-15):** `charts/postgres-cluster` legt nur ein
`username`/`password`-`Secret` im Namespace `application` an, kein fertiges `DATABASE_URL`, und
`reservation-worker` liegt (wie `charts/ticketflow-app`) im Namespace `ticketflow` (siehe
## Namespace-Aufteilung unten) —
Kubernetes-`Secret`s sind nicht Namespace-übergreifend referenzierbar. Gelöst nach demselben
Muster wie `catalog-api`/`notification-worker` (Niklas' Vorgabe, siehe deren Chart-Kommentare):
der Chart legt sein Postgres-Credentials-`Secret` **selbst** an
(`templates/secret.yaml`, `databaseSecret.{create,username,password}`, Lab-Zugangsdaten fest im
Repo hinterlegt statt `CHANGE_ME` – müssen mit `charts/postgres-cluster` `values.yaml`
`database.owner`/`.password` übereinstimmen), statt ein extern kopiertes Secret zu referenzieren.
Die Anwendung baut ihren Connection-String selbst aus `database.{host,port,name,sslMode}`
(ConfigMap) + diesem Secret zusammen (Keys `username`/`password`, explizit auf `DB_USER`/
`DB_PASSWORD` gemappt statt per `envFrom`, da die Schlüsselnamen nicht zu den erwarteten
Env-Var-Namen passen).

## Addon-Charts

Alle Cluster-weiten Operatoren/Addons (Longhorn, cert-manager, external-dns, CNPG-Operator,
Strimzi-Operator, kube-prometheus-stack, Loki, Tempo, AKHQ; künftig KEDA) liegen als **eigene,
kleine Wrapper-Charts** hier im Ordner, jeweils mit einer eigenen ArgoCD-`Application` in
[`../gitops/applicationset.yaml`](../gitops/applicationset.yaml), die auf `charts/<name>` in diesem Repo zeigt (nicht direkt
auf den Upstream-Helm-Repo) – ein einheitliches Muster für alle Charts in diesem Repo statt zwei
verschiedener (Application-inline-Values vs. lokaler Chart), und alle Werte landen sichtbar
versioniert in einer eigenen `values.yaml` statt in der Application eingebettet.

Zwei Varianten:

- **Wrapper um einen Upstream-Chart** ([`longhorn/`](longhorn/), [`cert-manager/`](cert-manager/),
  [`external-dns/`](external-dns/), [`cnpg-operator/`](cnpg-operator/),
  [`strimzi-operator/`](strimzi-operator/), [`kube-prometheus-stack/`](kube-prometheus-stack/),
  [`loki/`](loki/), [`tempo/`](tempo/), [`akhq/`](akhq/), [`keda/`](keda/)): `Chart.yaml` deklariert den Upstream-Chart als
  `dependencies`-Eintrag (Name/Version/Repository), `values.yaml` reicht unsere Werte unter dem
  Dependency-Alias (z.B. `longhorn:`) durch. ArgoCD baut die Dependency beim Sync automatisch
  (`helm dependency build`) – kein `Chart.lock`/`charts/*.tgz` im Repo (gitignored, siehe
  `.gitignore`), damit sich Dependency-Version-Bumps auf den `Chart.yaml`-Eintrag beschränken.
  `cnpg-operator`/`strimzi-operator`/`kube-prometheus-stack` brauchen zusätzlich
  `ServerSideApply=true` in ihrer Application (siehe `../gitops/README.md` ## Struktur) – alle
  drei bündeln einzelne CRDs über dem 262144-Byte-Annotationslimit der Kubernetes-API.
  `kube-prometheus-stack`, `loki` und `tempo` bilden zusammen den Observability-Stack (siehe
  `../gitops/README.md` ## Observability-Stack) – Persistenz durchgehend über Longhorn.
- **Volleigene Custom Resources**, die es upstream nicht als Chart gibt – analog zur
  CNPG-`Cluster`-CR in `postgres-cluster/`, nur eben nicht Teil eines Anwendungs-Charts, weil sie zum
  Addon-Layer statt zur TicketFlow-Anwendung gehören:
  - [`cert-manager-config/`](cert-manager-config/): `ClusterIssuer` fürs ACME/DNS-01-Setup.
    Siehe [`../gitops/README.md`](../gitops/README.md) ## DNS/ACME für die nötige
    TSIG-Secret-Voraussetzung (das Secret selbst steht bewusst nicht im Repo).
  - [`argocd-ingress/`](argocd-ingress/): `Certificate` + Traefik `IngressRouteTCP`, macht die
    ArgoCD-UI über die echte DHBW-DNS-Zone erreichbar. Siehe
    [`../gitops/README.md`](../gitops/README.md) ## ArgoCD-UI öffentlich erreichbar.
  - [`traefik-config/`](traefik-config/): kein Wrapper- und kein Custom-Resource-Chart im obigen
    Sinn, sondern ein einzelnes `HelmChartConfig`-Objekt (`helm.cattle.io/v1`), das Werte in k3s'
    eigenes, gebündeltes Traefik-`HelmChart` mergt, ohne dessen `helm-controller` die Ownership
    streitig zu machen — gibt Traefiks `LoadBalancer`-Service einen stabilen, per external-dns
    veröffentlichten Hostnamen als rebuild-festes CNAME-Ziel für `argocd-ingress` & Co. Siehe
    [`../gitops/README.md`](../gitops/README.md) ## ArgoCD-UI öffentlich erreichbar.

Lokal nachvollziehen/testen (kein Cluster nötig):

```bash
helm dependency update charts/longhorn   # zieht den Upstream-Chart nach charts/longhorn/charts/
helm template charts/longhorn
```

## `apicurio` — volleigener Chart, kein Wrapper (ursprünglich anders geplant)

[`apicurio/`](apicurio/) deployt Apicurio Registry (JSON-Schema-Registry für die
Kafka-Event-Contracts `ReservationRequested`/`ReservationResult`, siehe
`kafka-cluster/files/schemas/`). War ursprünglich als weiterer Wrapper-Chart um
`eshepelyuk/apicurio-registry-helm` (`oci://ghcr.io/eshepelyuk/helm/apicurio-registry`) geplant —
dasselbe Muster wie unten. Dessen ghcr.io-OCI-Paket liefert aber **403 Forbidden** auf jeden
anonymen Pull-Versuch (verifiziert sowohl über `docker run ... helm dependency update` als auch
direkt gegen `https://ghcr.io/token?...`; ghcr.io selbst ist erreichbar, ein anderes, unabhängiges
Paket dort liefert 200), vermutlich eine Folge von dessen Archivierung am 2024-11-13 — das Paket
ist offenbar nicht mehr öffentlich, nicht nur unmaintained. Keine der geprüften Alternativen
(`apicurio.github.io/apicurio-registry-helm-chart` → 404, `touk/apicurio-registry` → erreichbar,
aber noch veralteter, appVersion max. `2.2.5.Final`, letztes Update 2022) war brauchbar; der
offizielle Weg wäre der Apicurio-Registry-Operator, der aber OLM voraussetzt — ein in diesem
Projekt bewusst nicht genutzter Mechanismus (CNPG/Strimzi installieren beide über reine
Helm-Charts, siehe oben). Deshalb: volleigener, schlanker Chart (Deployment/Service/ConfigMap,
`_helpers.tpl`, kein `values.schema.json` – Umfang zu klein), analog zu
[`cert-manager-config/`](cert-manager-config/)/[`argocd-ingress/`](argocd-ingress/) oben, nicht zu
den App-Charts (`notification-worker`/`reservation-worker`) mit ihren fünf Ressourcen.

**Zusätzlicher Fallstrick, der die Wrapper-Idee ohnehin erschwert hätte:** der
`eshepelyuk`-Chart ist für Apicurio Registry **2.x** gebaut, nicht 3.x — sein
`registry.image.persistenceSuffix`-Mechanismus hängt dem Image-*Repository* (nicht dem Tag) einen
Storage-Suffix an (`-kafkasql`/`-sql`/`-mem`, siehe dessen `_helpers.tpl`), weil 2.x pro
Storage-Variante ein eigenes Image veröffentlichte; die `registry.kafka`/`registry.sql`-Value-Blöcke
setzen entsprechend 2.x-Variablennamen (`KAFKA_BOOTSTRAP_SERVERS`, `REGISTRY_DATASOURCE_URL`).
Apicurio 3.x ist dagegen **ein** Image für alle Storage-Varianten, Auswahl über
`APICURIO_STORAGE_KIND`/`APICURIO_KAFKASQL_BOOTSTRAP_SERVERS` (verifiziert gegen die
3.1.x-Doku "Installing Apicurio Registry using Docker") — selbst mit einem funktionierenden
ghcr.io-Zugriff hätte der Wrapper diese Variablen von Hand über `extraEnv` setzen und
`persistenceSuffix: false` erzwingen müssen. Der jetzige eigene Chart setzt sie direkt (siehe
`apicurio/values.yaml` → `config.storageKind`/`config.kafkaBootstrapServers`, gerendert in
`templates/configmap.yaml`).

Storage-Backend: `kafkasql` (Schemas landen in einem Kafka-Topic + In-Memory-H2 statt einer
eigenen Postgres-Instanz) — vermeidet bewusst dieselbe Cross-Namespace-Secret-Problematik, die
aktuell `reservation-worker`s echten Deploy blockiert (siehe oben). Kein bestätigter
Health-Endpoint für 3.x gefunden (evtl. separater Management-Port statt 8080) — Liveness-/
Readiness-Probes nutzen deshalb die echte REST-API (`GET /apis/registry/v3/groups`) statt eines
geratenen Health-Pfads; dieselbe Begründung gilt für die Wartelogik im
`schema-registrar`-Job in `ticketflow/templates/schema-registry/`. Noch kein Sync live verifiziert (kein Cluster verfügbar) — `helm lint`/
`template` lokal sauber.

## Resource-Limits, Security-Context, NetworkPolicy

Alle acht Wrapper-Charts oben setzen durchgehend `resources.requests`/`resources.limits`
(Werte klein für die Lab-VMs, siehe die einzelnen `values.yaml`-Kommentare) und aktivieren die
Chart-eigene `networkPolicy`, wo der Upstream-Chart eine anbietet und sie sich als sicher
erwiesen hat (cert-manager, kube-prometheus-stack, Tempo — external-dns/CNPG/Strimzi/Longhorn
haben kein natives NetworkPolicy-Feld, dafür wurde bewusst keine eigene NetworkPolicy von Hand
nachgebaut, siehe unten). **Loki ist die eine Ausnahme: `networkPolicy.enabled: false`,
bewusst NICHT aktiviert** — der Chart legt bei `enabled: true` zusätzlich ungefragt eine
`<release>-namespace-only`-Policy mit **leerem `podSelector: {}`** an
(`templates/networkpolicy.yaml`), die dann für **alle** Pods im Namespace `monitoring` gilt,
nicht nur für Loki selbst, und nur noch Traffic innerhalb der Namespace-Grenze erlaubt. Live
beobachtet: das hat den Grafana-Sidecars (selber Namespace, aus `charts/kube-prometheus-stack`)
den Egress zum Kubernetes-API-Server gekappt ("connection refused" auf die
Service-ClusterIP) → CrashLoopBackOff. Kein Chart-eigenes Sub-Flag, um nur diese eine Policy
abzuschalten, deshalb für Loki komplett aus. Tempos NetworkPolicy hat dasselbe Problem nicht
(dort korrekt über `podSelector.matchLabels` auf die eigenen Pods beschränkt) — **bei jedem
neuen Chart erst den gerenderten NetworkPolicy-`podSelector` prüfen, bevor `enabled: true`
gesetzt wird**, ein leerer/fehlender Selector auf einer geteilten Namespace ist der
Symptomkiller.

Ein zweiter, unabhängiger Fehler kam aus derselben Hardening-Runde: `grafana.persistence.enabled`
plus der Chart-Default `deploymentStrategy.type: RollingUpdate` deadlockt live bei **jeder**
künftigen Änderung am Grafana-Pod-Template (auch am `resources`-Block hier) – der neue Pod kann
die einzelne `ReadWriteOnce`-PVC nicht attachen, solange der alte Pod sie noch hält
("Multi-Attach error"), Kubernetes killt den alten Pod aber erst, wenn der neue `Ready` ist.
Deshalb `charts/kube-prometheus-stack/values.yaml` → `grafana.deploymentStrategy.type: Recreate`
– kurze Grafana-Downtime beim Rollout statt eines dauerhaften Deadlocks. Dieselbe Falle gilt für
jeden künftigen Chart mit `persistence.enabled` + Deployment (nicht StatefulSet) + einem
RWO-Volume: **immer `deploymentStrategy: Recreate` setzen, sobald persistente Einzel-Replica-PVC
+ `RollingUpdate`-Default zusammentreffen.**

Bei `securityContext`/`podSecurityContext` gilt: **erst prüfen, dann setzen**. Die meisten
dieser Charts (cert-manager, external-dns, CNPG, kube-prometheus-stack inkl. aller
Subcharts, Loki) liefern bereits einen vollständig gehärteten Default (`runAsNonRoot`,
`readOnlyRootFilesystem`, `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault`) — dort
wurde bewusst **nichts überschrieben**, nur per Kommentar in der jeweiligen `values.yaml`
vermerkt, dass das schon passt. Explizit ergänzt wurde nur, wo der Chart-Default tatsächlich
leer war (Strimzi: `podSecurityContext`/`securityContext` beide `{}`) oder ein Feld fehlte
(Tempo: `allowPrivilegeEscalation`/`capabilities.drop` fehlten am Container, `readOnlyRootFilesystem`
bewusst NICHT gesetzt — der Chart warnt im eigenen values.yaml-Kommentar "fails if true, do not
enable"). **Longhorn** hat als einziger Chart gar kein Security-Context-Feld — Longhorn-Manager/
-Driver brauchen privilegierten Host-Zugriff (iSCSI, Blockgeräte) und lassen sich strukturell
nicht auf ein "restricted" Profil härten; das ist eine dokumentierte, unvermeidbare Ausnahme,
keine Unterlassung.

Ein Stolperstein beim Editieren, der einmal live aufgetreten ist: manche Charts trennen
Pod-Level- und Container-Level-`securityContext` in **zwei gleichnamig wirkende, aber
unterschiedlich verschachtelte** Felder (z.B. bei Tempo: `tempo.tempo.securityContext` landet
im Container, `tempo.securityContext` im Pod — siehe `templates/statefulset.yaml` im
Upstream-Chart). `allowPrivilegeEscalation`/`capabilities` gehören ausschließlich in den
Container-Level-Block; im Pod-Level-Block sind das ungültige Felder (Kubernetes' `PodSecurityContext`-
Typ kennt sie nicht). Vor jedem neuen `securityContext`-Override im Upstream-Chart-Template
nachsehen, welches Values-Feld wirklich wohin gemappt wird, dann mit `helm template` +
`python3 -c "...c.get('securityContext')..."` (Pod **und** Container getrennt ausgeben)
gegenprüfen, statt der Struktur eines anderen Charts blind zu folgen.

NetworkPolicies wurden bewusst **nicht** auf konkrete Pod-/Namespace-Selektoren verengt
(Chart-Default-Regeln reichen, `enabled: true` genügt) — die genauen Cross-Namespace-Verkehrsflüsse
(z.B. welche Namespaces Prometheus scrapen darf) stehen erst fest, sobald
`charts/kafka-cluster` existiert; eine zu früh verengte Policy würde sonst später stillschweigend
Metrics-/Log-/Trace-Sammlung blockieren, ohne dass ein Fehler sichtbar wird.
