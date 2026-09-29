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
    └── poddisruptionbudget.yaml
```

Feste Key-Reihenfolge in `values.yaml` (je Komponente, falls der Chart mehrere rendert):
`image`, `replicaCount`, `resources`, `podSecurityContext`, `securityContext`, `config`,
`database`/`secret`, `service`, `ingress`, `serviceMonitor`/`podMonitor`,
`podDisruptionBudget`, `topologySpreadConstraint`.

Keine `networkpolicy.yaml` im App-Chart: alle NetworkPolicies stehen zentral in
`charts/network-policies` (siehe ## network-policies) — eine neue Verbindung eines Services wird
dort freigegeben.

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
| `ticketflow` | `ticketflow-app` (catalog-api + frontend), `reservation-worker`, `notification-worker`, **`postgres-cluster`** |
| `kafka` | `strimzi-operator`, `kafka-cluster`, `apicurio`, `akhq` |
| `openbao` | `openbao`, `openbao-postgres` (B1, eigene CNPG-Cluster-CR für OpenBaos Storage-Backend) |
| `external-secrets` | `external-secrets-operator`, `openbao-secretstore` |
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
(Deployment, ConfigMap, Secret, PodDisruptionBudget, PodMonitor)
deutlich mehr rendert als die anderen Custom-Charts. Bewusst kein `Service` — es gibt keinen
anderen Konsumenten der HTTP-Endpunkte (reiner Kafka-Consumer, Kubelet-Probes gehen direkt an den
Pod), daher scraped ein `PodMonitor` (statt `ServiceMonitor`) `/metrics` direkt über den
Pod-Label-Selector, siehe `notification-worker/templates/podmonitor.yaml`. Default sind `replicaCount: 3` + `PodDisruptionBudget`. Seit der Umstellung auf das ApplicationSet ist er als Element
`notification-worker` mit ausgerollt — vorher fehlte ihm eine Application, und `image.tag` war
leer, sodass ein Deploy ohne `--set` gar nicht möglich war. 

## `reservation-worker` — eigenständiger Chart, gleiches Muster wie `notification-worker`

[`reservation-worker/`](reservation-worker/) ist der zweite vollständig implementierte Service,
analog zu `notification-worker` als eigener Chart (Deployment, ConfigMap, Secret,
PodDisruptionBudget, PodMonitor, `_helpers.tpl`, eigenes
`values.schema.json`) — deployt als Element `reservation-worker`. Abweichungen von
`notification-worker`s Vorlage: `replicaCount: 1` statt `3` — der Service wird von KEDA auf
Kafka-Consumer-Lag skaliert (P6, Element `keda`), ein hoher statischer Default wäre daneben
irreführend; und ein zusätzlicher `database`-Block statt
`notification-worker`s SMTP/TLS-Secret-Feldern, weil der Service transaktional in CNPG-Postgres
schreibt (siehe `src/reservation-worker/README.md` ## Zweck).

**Postgres-Anbindung (vereinfacht 2026-09-17, Credential-Quelle geändert 2026-09-23):** Der
CNPG-Cluster lag ursprünglich im eigenen Namespace `application`, weshalb `catalog-api` und
`reservation-worker` sein `username`/`password`-`Secret` nicht per `secretKeyRef` lesen konnten
(keine Namespace-Grenzen) und beide Charts die Zugangsdaten stattdessen in ein **eigenes** Secret
duplizierten — dieselben Werte an drei Stellen, die von Hand übereinstimmen mussten. Seit
`charts/postgres-cluster` im selben Namespace wie die Anwendung läuft, entfällt das: beide Charts
referenzieren direkt das von dort angelegte `<ticketflow.postgres.databaseName>-postgres-user`
(Keys `username`/`password`). Die chart-eigenen `secret.yaml`-Templates und die
`databaseSecret`-Werteblöcke sind entfallen. **Das Passwort steht seit B1 (Root-README ## Bonus)
nirgends mehr im Repo** — die CNPG-`Cluster`-CR gibt bewusst kein eigenes `bootstrap.initdb.secret`
mehr vor, CNPG generiert das Credential selbst (`<clusterName>-app`), ein `PushSecret` spiegelt es
nach OpenBao und ein `ExternalSecret` synct es von dort unter dem oben genannten konventionellen
Namen zurück — siehe `charts/README.md`-Abschnitt zu `openbao-postgres`/`openbao-secretstore`
weiter unten und `CLAUDE.md` ## Current implementation status für Details.

Host, Datenbankname und Secret-Name leiten sich aus `ticketflow.postgres.databaseName` ab
(`<name>-postgres-cluster-rw` bzw. `<name>-postgres-user`), `database.{host,name,credentialsSecretName}`
bleiben als Override leer.

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
  [`loki/`](loki/), [`tempo/`](tempo/), [`akhq/`](akhq/), [`keda/`](keda/), [`openbao/`](openbao/),
  [`external-secrets-operator/`](external-secrets-operator/)): `Chart.yaml` deklariert den
  Upstream-Chart als
  `dependencies`-Eintrag (Name/Version/Repository), `values.yaml` reicht unsere Werte unter dem
  Dependency-Alias (z.B. `longhorn:`) durch. ArgoCD baut die Dependency beim Sync automatisch
  (`helm dependency build`) – kein `Chart.lock`/`charts/*.tgz` im Repo (gitignored, siehe
  `.gitignore`), damit sich Dependency-Version-Bumps auf den `Chart.yaml`-Eintrag beschränken.
  `cnpg-operator`/`strimzi-operator`/`kube-prometheus-stack`/`external-secrets-operator` brauchen
  zusätzlich `ServerSideApply=true` in ihrer Application (siehe `../gitops/README.md` ##
  Struktur) – alle vier bündeln einzelne CRDs über (oder, bei `external-secrets-operator`, knapp
  unter, aber als JSON-kodierte Annotation potenziell über) dem 262144-Byte-Annotationslimit der
  Kubernetes-API. `kube-prometheus-stack`, `loki` und `tempo` bilden zusammen den
  Observability-Stack (siehe `../gitops/README.md` ## Observability-Stack) – Persistenz
  durchgehend über Longhorn. `openbao` (B1, Root-README ## Bonus) ist der einzige Wrapper-Chart
  mit einem eigenen, zusätzlichen Template (`templates/bootstrap-job.yaml` +
  `bootstrap-rbac.yaml`, außerhalb des `openbao:`-Dependency-Alias) – ein
  `post-install,post-upgrade`-Hook-Job, der OpenBao initialisiert/entsiegelt und mit den
  Kubernetes-Auth-Voraussetzungen für `external-secrets-operator` befüllt, siehe die Kommentare
  in beiden Dateien und `../CLAUDE.md` ## Current implementation status.
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
  - [`openbao-postgres/`](openbao-postgres/): eine zweite, von `postgres-cluster/` komplett
    getrennte CNPG-`Cluster`-CR – OpenBaos eigenes PostgreSQL-Storage-Backend (B1), eigener
    Namespace `openbao`. Anders als `postgres-cluster/` legt sie **kein** eigenes
    Credentials-Secret an (`bootstrap.initdb` ohne `secret`-Feld) – CNPG generiert das
    App-User-Credential selbst, weil OpenBao sein eigenes Storage-Backend-Passwort nicht aus sich
    selbst lesen kann, bevor es entsiegelt ist.
  - [`openbao-secretstore/`](openbao-secretstore/): eine einzelne `ClusterSecretStore`-CR gegen
    OpenBao (`spec.provider.vault`), die jeder migrierte `ExternalSecret` in diesem Repo
    referenziert (siehe `charts/postgres-cluster`, `charts/kafka-cluster`, `charts/akhq`,
    `charts/notification-worker`).

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
(Werte klein für die Lab-VMs, siehe die einzelnen `values.yaml`-Kommentare). „Durchgehend“ heißt
auch Sidecars, Init-Container und Hook-Jobs, die ein Chart mit eigenem Values-Feld neben dem
Hauptcontainer rendert (Grafana-/Loki-`sidecar`, `initChownData`, `admissionWebhooks.patch`,
MinIO-`make*Job`) — die fielen bei einer Prüfung des **laufenden** Clusters auf
(`kubectl get pods -A -o json`, jeden Container auf `requests`/`limits` für `cpu`/`memory`
prüfen), nicht bei `helm lint`. Zwei Sorten Container tauchen im gerenderten Chart gar nicht auf
und brauchen einen Umweg: der `config-reloader`, den der Prometheus-Operator selbst in
Prometheus/Alertmanager einfügt (`prometheusOperator.prometheusConfigReloader.resources` → wird als
`--config-reloader-*`-Flag an den Operator übergeben), und die CSI-Pods, die Longhorn zur Laufzeit
anlegt (`defaultSettings.systemManagedCSIComponentsResourceLimits`, ein JSON-String). Was danach
übrig bleibt, hat kein Feld im Upstream-Chart, siehe Root-`README.md` ## Grenzen.

Die Chart-eigenen `networkPolicy`-Schalter sind in **allen** Charts aus — sämtliche
NetworkPolicies kommen aus `charts/network-policies` (siehe ## network-policies unten). Die
Upstream-Policies waren vorher teils zu offen (AKHQ/Tempo: jede Quelle; cert-manager: Egress zu
beliebigen Zielen, Webhook/cainjector gar nicht erfasst), teils kaputt (kube-prometheus-stack:
Prometheus komplett abgeschottet, s.u.). Die folgenden zwei Vorfälle sind der Grund, warum ein
Upstream-Schalter nie ungeprüft an geht. **Loki:** der Chart legt bei `enabled: true` zusätzlich ungefragt eine
`<release>-namespace-only`-Policy mit **leerem `podSelector: {}`** an
(`templates/networkpolicy.yaml`), die dann für **alle** Pods im Namespace `monitoring` gilt,
nicht nur für Loki selbst, und nur noch Traffic innerhalb der Namespace-Grenze erlaubt. Live
beobachtet: das hat den Grafana-Sidecars (selber Namespace, aus `charts/kube-prometheus-stack`)
den Egress zum Kubernetes-API-Server gekappt ("connection refused" auf die
Service-ClusterIP) → CrashLoopBackOff. Kein Chart-eigenes Sub-Flag, um nur diese eine Policy
abzuschalten, deshalb für Loki komplett aus. **kube-prometheus-stack:** die Prometheus-Policy
des Charts hatte live `policyTypes: [Ingress, Egress]` ohne eine einzige Regel — Prometheus
erreichte weder den API-Server (Service Discovery) noch ein Scrape-Ziel, `/api/v1/targets`
lieferte 0 aktive Targets, ohne dass irgendwo ein Fehler auffiel. Lehre aus beiden: **`helm lint`
sagt über NetworkPolicies nichts** — gerendertes Ergebnis lesen und gegen echten Traffic prüfen.

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

## network-policies

Fully-custom Chart, deployt als Element `network-policies` (Ziel-Namespace nur formal, jedes
Objekt trägt seinen eigenen). Enthält **alle** NetworkPolicies des Clusters — Ausnahme sind nur
die Ingress-Policies, die ArgoCDs Install-Manifest (Ansible) selbst mitbringt; die bleiben, der
Chart ergänzt dort default-deny und den Egress.

**Aufbau.** Jeder Namespace (auch `kube-system`, `argocd`, `default`, `kube-public`,
`kube-node-lease`) bekommt `default-deny-all` (Ingress + Egress, der einzige gewollt leere
`podSelector`) und `allow-dns-egress` zu CoreDNS. Danach je Komponente eine `allow-<name>`-Policy,
die genau ihre Verbindungen freigibt — eine Datei pro Namespace-Gruppe unter `templates/`
(`app.yaml`, `kafka.yaml`, `openbao.yaml`, `external-secrets.yaml`, `operators.yaml`,
`monitoring.yaml`, `platform.yaml`), die wiederkehrenden Bausteine (API-Server-Egress,
Prometheus-/Traefik-/API-Server-Peers, CNPG-Cluster) in `_helpers.tpl`. Namespaces, Kafka-/
Postgres-Clusternamen, Node-Subnetze und der RFC2136-Server kommen aus `gitops/env/cluster.yaml`.

**Drei k3s-Eigenheiten, ohne die nichts funktioniert** (live auf den Nodes geprüft):

- Der API-Server ist kein Pod. Pods erreichen ihn über die ClusterIP des Service `kubernetes`,
  kube-proxy übersetzt das **vor** der Policy-Auswertung auf `<Control-Plane-IP>:6443` — Egress
  also per `ipBlock` auf `ticketflow.network.nodeCIDRs`, Port 6443 (nicht 443).
- Ruft der API-Server einen Pod auf einem **anderen** Node auf (Admission-Webhooks, APIServices,
  `kubectl cnpg status`), ist die Quelladresse die Flannel-Tunneladresse des Control-Plane-Nodes
  (`fd00:42::`/`10.42.0.0`), nicht die Node-IP — `ticketflow.network.controlPlaneTunnelIPs`.
- kube-router (k3s' Policy-Controller) lässt Traffic vom **lokalen** Node immer durch („permit
  traffic to pods when source is the pod's local node“). Deshalb brauchen Kubelet-Probes, iSCSI
  zu Longhorns Instance-Manager und Hairpin-Traffic keine Regel.

**Operator-Vorgaben aus der offiziellen Doku:**

- *CloudNativePG* (`docs/src/networking.md`, `samples/networkpolicy-example.yaml`): der Operator
  muss jede Instanz auf 8000 und 5432 erreichen, die Instanzen untereinander. Zusätzlich braucht
  jede Instanz den API-Server (der Instance-Manager ist selbst ein Controller) —
  `np.cnpgCluster` in `_helpers.tpl`, genutzt für App- und OpenBao-Datenbank.
- *Strimzi* (Restricting access to listeners with network policies): der Operator erzeugt je
  Listener eine Policy „von allen Namespaces“, damit wären 9092 (SCRAM) und vor allem 9093
  (anonym, Superuser, nur für Apicurio) clusterweit offen. `STRIMZI_NETWORK_POLICY_GENERATION=false`
  in `charts/strimzi-operator` ist der dokumentierte Schalter für eigene Policies; die internen
  Regeln (9090 KRaft-Quorum, 9091 Replikation, 8443 Kafka-Agent) sind 1:1 die zuvor vom Operator
  erzeugten, die Client-Listener auf die tatsächlichen Clients verengt. Strimzi-Pods brauchen
  außerdem den API-Server: der `strimzisecrets`-Config-Provider liest Zertifikate zur Laufzeit
  über die API.

**Bewusste Grobheiten**, jeweils mit Begründung im Template: Prometheus darf Egress zu allen Pods
(die Ingress-Seite jedes Ziels regelt den Metrik-Port, eine zweite Portliste würde nur veralten);
`longhorn-system` erlaubt Verkehr innerhalb des Namespaces pauschal (Engine↔Replika über
dynamisch vergebene Ports); Internet-Egress gibt es nur portbeschränkt und nur für cert-manager
(ACME 443, DNS 53), CoreDNS (Upstream 53), ArgoCD-Repo-Server (22/443) und den
Schema-Registrierungs-Job (`apk add`, 80/443). Longhorns Upgrade-Checker ist deshalb abgeschaltet
(`charts/longhorn`), Grafana/Loki/Tempo-Nutzungsstatistiken laufen still ins Leere.

**Wie die Verkehrsmatrix ermittelt und geprüft wurde.** Aus dem Live-Cluster: Pods/Labels/Ports,
Services, alle Validating-/MutatingWebhookConfigurations und APIServices (= Aufrufe vom
API-Server), alle ServiceMonitor/PodMonitor-Ziele, alle Ingress-Backends. Danach zwei
Simulationen gegen den gerenderten Chart (NetworkPolicy-Semantik inkl. der kube-router-Regel für
lokalen Traffic): (1) jede tatsächlich offene TCP-Verbindung, per `ss` in jedem Pod-Netns und auf
jedem Host über eine Minute mitgeschnitten — 89 Flow-Typen, 0 blockiert; (2) 118 aus der
Konfiguration abgeleitete Soll-Flows (jeder Scrape, jeder Webhook, jedes Ingress-Backend, DNS
für jeden Pod) — 0 blockiert. Gegenprobe mit nur default-deny: 70 der 89 Flows blockiert, die
Simulation erkennt Sperren also. UDP wurde nicht mitgeschnitten (einziger UDP-Verkehr: DNS und
Alertmanager-Mesh, beide explizit geregelt).

**Einmalige Schritte beim ersten Rollout auf einem bestehenden Cluster:** Strimzi räumt seine
bisher generierten Policies nicht selbst ab — danach
`kubectl -n kafka delete networkpolicy ticketflow-kafka-network-policy-kafka ticketflow-kafka-entity-operator ticketflow-kafka-kafka-exporter`
(sonst bleiben 9092/9093 über diese alten Policies weiter offen). Die übrigen alten Policies
(AKHQ, Tempo, kube-prometheus-stack, cert-manager, CNPG) entfernt ArgoCD per `prune` selbst.

**Neue Verbindung, neuer Service:** Regel hier ergänzen, sonst wird der Verkehr verworfen — ohne
Fehlermeldung, nur als Timeout. **Notausgang**, falls ArgoCD sich selbst ausgesperrt hat:
`kubectl -n argocd delete networkpolicy -l app.kubernetes.io/name=network-policies`.
