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
    ├── servicemonitor.yaml            # mit Service; sonst podmonitor.yaml (beide derzeit aus)
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
  ArgoCD-Application (`destination.namespace` in `gitops/applicationsets/`).
- **`securityContext` kommt aus `values.yaml`**, nie fest aus dem Template.
- **Probes `/healthz` (liveness) und `/readyz` (readiness)**; `/metrics` wird bereitgestellt
  (Service-/PodMonitor sind aus, solange kein Prometheus im Cluster läuft).
- **Mindestens zwei Replicas, hart auf zwei Regionen verteilt** (P4): `topologySpreadConstraints`
  mit `whenUnsatisfiable: DoNotSchedule`, `maxSkew: 1` und `matchLabelKeys: [pod-template-hash]`,
  dazu ein PodDisruptionBudget — auch im dev-Profil.
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
| `longhorn-system`, `cert-manager`, `external-dns`, `cnpg-system`, `keda` | die jeweiligen Addons |

Beide App-Namen stehen in `gitops/env/cluster.yaml` (`ticketflow.namespaces.app`/`.kafka`); jeder
Chart, der den Kafka-Bootstrap-Service oder die Apicurio-Registry braucht, leitet die Adresse
daraus ab (`<clusterName>-kafka-bootstrap.<kafkaNs>.svc:9092` bzw.
`http://apicurio.<kafkaNs>.svc:8080/...`) statt sie auszuschreiben.

**Eine Ausnahme, die man kennen muss:** `charts/kafka-cluster` rendert seine `KafkaUser`-CRs
nicht in den Kafka-, sondern in den App-Namespace. Der Strimzi User Operator legt das generierte
SCRAM-`Secret` immer im Namespace der CR ab, und `secretKeyRef` überschreitet keine
Namespace-Grenze — die Services kämen sonst nicht an ihr Passwort. Dafür setzt der Chart
`spec.entityOperator.userOperator.watchedNamespace`; siehe `kafka-cluster/README.md` ## Namespaces.

`charts/akhq` ist der einzige Client, der den Bootstrap-Service nicht ableitet, sondern als
kurzen In-Namespace-Namen ausschreibt: sein Wert liegt in den Values des gewrappten
Upstream-Charts, und AKHQ läuft ohnehin im selben Namespace wie der Cluster.

## Zentrale Werte: `gitops/env/cluster.yaml`

Image-Registry, Image-Tag, DNS-Zone, ACME-Kontakt und TSIG-Key-Name stehen in **keinem** Chart,
sondern einmalig in [`../gitops/env/cluster.yaml`](../gitops/env/cluster.yaml). ArgoCD reicht die
Datei jedem Chart per Multi-Source `$values` als zusätzliche Values-Datei durch (siehe
`../gitops/applicationsets/`); die Charts lesen sie als `.Values.ticketflow.*`.

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
in `../gitops/applicationsets/`), damit ein Sync/Rollback des einen den
anderen nicht anfasst. `ticketflow-app` läuft im Namespace `ticketflow`, `kafka-cluster` in
`kafka`; `catalog-api` leitet die Bootstrap-Adresse aus `gitops/env/cluster.yaml` ab.

## `notification-worker` und `reservation-worker` — je ein eigener Chart

[`notification-worker/`](notification-worker/) und [`reservation-worker/`](reservation-worker/)
sind volleigene Charts (keine Upstream-`dependencies`) mit `_helpers.tpl` und eigenem
`values.schema.json`, deployt als gleichnamige ApplicationSet-Elemente. Bewusst kein `Service` —
es gibt keinen anderen Konsumenten der HTTP-Endpunkte (reine Kafka-Consumer, Kubelet-Probes gehen
direkt an den Pod); ein `PodMonitor` statt `ServiceMonitor` ist vorbereitet, aber aus.

Unterschiede: `reservation-worker` wird von KEDA auf den Consumer-Lag skaliert (Bonus B2,
2–10 Pods, `templates/keda-scaledobject.yaml`; `replicaCount` ist nur der Startwert, ArgoCD
ignoriert `spec.replicas`) und hat einen `database`-Block, weil er transaktional in
CNPG-Postgres schreibt. `notification-worker` hat feste Replicas (dev 2, prod 3) und optionale
SMTP-/Kafka-mTLS-Secrets aus OpenBao (siehe `notification-worker/README.md`).

**Postgres-Anbindung:** `charts/postgres-cluster` läuft im selben Namespace wie die Anwendung,
deshalb referenzieren `catalog-api` und `reservation-worker` direkt das von CNPG angelegte Secret
`<ticketflow.postgres.databaseName>-postgres-cluster-app` (Keys `username`/`password`) — kein
Duplikat im Chart. **Das Passwort steht nirgends im Repo:** die CNPG-`Cluster`-CR gibt kein
`bootstrap.initdb.secret` vor, CNPG generiert das Credential selbst, und ein `PushSecret`
spiegelt es nur als Audit-Kopie nach OpenBao (B1).

Host, Datenbankname und Secret-Name leiten sich aus `ticketflow.postgres.databaseName` ab
(`<name>-postgres-cluster-rw` bzw. `<name>-postgres-cluster-app`), `database.{host,name,credentialsSecretName}`
bleiben als Override leer.

## Addon-Charts

Alle Cluster-weiten Operatoren/Addons (Longhorn, cert-manager, external-dns, CNPG-Operator,
Strimzi-Operator, KEDA, External Secrets, OpenBao, AKHQ) liegen als **eigene,
kleine Wrapper-Charts** hier im Ordner, jeweils mit einer eigenen ArgoCD-`Application` in
[`../gitops/applicationsets/`](../gitops/applicationsets/), die auf `charts/<name>` in diesem Repo zeigt (nicht direkt
auf den Upstream-Helm-Repo) – ein einheitliches Muster für alle Charts in diesem Repo statt zwei
verschiedener (Application-inline-Values vs. lokaler Chart), und alle Werte landen sichtbar
versioniert in einer eigenen `values.yaml` statt in der Application eingebettet.

Zwei Varianten:

- **Wrapper um einen Upstream-Chart** ([`longhorn/`](longhorn/), [`cert-manager/`](cert-manager/),
  [`external-dns/`](external-dns/), [`cnpg-operator/`](cnpg-operator/),
  [`strimzi-operator/`](strimzi-operator/), [`akhq/`](akhq/), [`keda/`](keda/), [`openbao/`](openbao/),
  [`external-secrets-operator/`](external-secrets-operator/)): `Chart.yaml` deklariert den
  Upstream-Chart als
  `dependencies`-Eintrag (Name/Version/Repository), `values.yaml` reicht unsere Werte unter dem
  Dependency-Alias (z.B. `longhorn:`) durch. ArgoCD baut die Dependency beim Sync automatisch
  (`helm dependency build`) – kein `Chart.lock`/`charts/*.tgz` im Repo (gitignored, siehe
  `.gitignore`), damit sich Dependency-Version-Bumps auf den `Chart.yaml`-Eintrag beschränken.
  `cnpg-operator`/`strimzi-operator`/`keda`/`external-secrets-operator` brauchen
  zusätzlich `ServerSideApply=true` in ihrer Application (siehe `../gitops/README.md` ##
  Struktur) – alle vier bündeln einzelne CRDs über (oder, bei `external-secrets-operator`, knapp
  unter, aber als JSON-kodierte Annotation potenziell über) dem 262144-Byte-Annotationslimit der
  Kubernetes-API. `openbao` (B1, Root-README ## Bonus) ist der einzige Wrapper-Chart
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
    OpenBao (`spec.provider.vault`), die alle `ExternalSecret`s (`charts/akhq`,
    `charts/notification-worker`) und `PushSecret`s (`charts/postgres-cluster`,
    `charts/kafka-cluster`) referenzieren.

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
dasselbe Muster wie oben. Dessen ghcr.io-OCI-Paket liefert aber **403 Forbidden** auf jeden
anonymen Pull-Versuch (verifiziert sowohl über `docker run ... helm dependency update` als auch
direkt gegen `https://ghcr.io/token?...`; ghcr.io selbst ist erreichbar, ein anderes, unabhängiges
Paket dort liefert 200), vermutlich eine Folge von dessen Archivierung am 2024-11-13 — das Paket
ist offenbar nicht mehr öffentlich, nicht nur unmaintained. Keine der geprüften Alternativen
(`apicurio.github.io/apicurio-registry-helm-chart` → 404, `touk/apicurio-registry` → erreichbar,
aber noch veralteter, appVersion max. `2.2.5.Final`, letztes Update 2022) war brauchbar; der
offizielle Weg wäre der Apicurio-Registry-Operator, der aber OLM voraussetzt — ein in diesem
Projekt bewusst nicht genutzter Mechanismus (CNPG/Strimzi installieren beide über reine
Helm-Charts, siehe oben). Deshalb: volleigener, schlanker Chart (Deployment/Service/ConfigMap/
PodDisruptionBudget, `_helpers.tpl`, `values.schema.json`), analog zu
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
eigenen Postgres-Instanz); zwei Replicas lesen das Journal je mit eigener Consumer-Group. Apicurio
verbindet sich über den unauthentifizierten Listener 9093, weil 3.3.3 für Kafka nur OAuth, kein
SCRAM kann (siehe `charts/kafka-cluster`). Kein bestätigter Health-Endpoint für 3.x gefunden —
Liveness-/Readiness-Probes nutzen deshalb die echte REST-API (`GET /apis/registry/v3/groups`);
dieselbe Begründung gilt für die Wartelogik im Registrierungs-Job in
`kafka-cluster/templates/schema-registry/`.

## Resource-Limits, Security-Context, NetworkPolicy

Alle Wrapper-Charts setzen durchgehend `resources.requests`/`resources.limits` (Werte klein für
die Lab-VMs, siehe die einzelnen `values.yaml`-Kommentare) — auch für Sidecars, Init-Container und
Hook-Jobs, die ein Chart mit eigenem Values-Feld rendert. Geprüft wird das am **laufenden** Cluster
(`kubectl get pods -A -o json`, jeden Container auf `requests`/`limits` prüfen), nicht mit
`helm lint`. Container, die im gerenderten Chart gar nicht auftauchen, brauchen einen Umweg, z. B.
die CSI-Pods, die Longhorn zur Laufzeit anlegt (`defaultSettings.systemManagedCSIComponentsResourceLimits`,
ein JSON-String). Was danach übrig bleibt, hat kein Feld im Upstream-Chart, siehe Root-`README.md` ## Grenzen.

Die Chart-eigenen `networkPolicy`-Schalter sind in **allen** Charts aus — sämtliche
NetworkPolicies kommen aus `charts/network-policies` (siehe unten). Upstream-Policies waren teils
zu offen (AKHQ: jede Quelle; cert-manager: Egress zu beliebigen Zielen), teils kaputt: eine legte
eine Policy mit **leerem `podSelector: {}`** an, die damit für **alle** Pods im Namespace galt und
fremden Pods den API-Server kappte; eine andere hatte `policyTypes: [Ingress, Egress]` ohne eine
einzige Regel. Lehre: **`helm lint` sagt über NetworkPolicies nichts** — gerendertes Ergebnis lesen
und gegen echten Traffic prüfen.

Bei `persistence.enabled` + Deployment (nicht StatefulSet) + einer einzelnen `ReadWriteOnce`-PVC
deadlockt der Default `RollingUpdate` bei jeder Änderung am Pod-Template („Multi-Attach error“) —
dort immer `strategy: Recreate` setzen.

Bei `securityContext`/`podSecurityContext` gilt: **erst prüfen, dann setzen**. Die meisten Charts
(cert-manager, external-dns, CNPG, KEDA, External Secrets) liefern bereits einen vollständig
gehärteten Default (`runAsNonRoot`, `readOnlyRootFilesystem`, `capabilities.drop: [ALL]`,
`seccompProfile: RuntimeDefault`) — dort wurde bewusst **nichts überschrieben**. Explizit ergänzt
wurde nur, wo der Default leer war (Strimzi, AKHQ). **Longhorn** hat als einziger Chart gar kein
Security-Context-Feld — Longhorn-Manager/-Driver brauchen privilegierten Host-Zugriff (iSCSI,
Blockgeräte); eine dokumentierte, unvermeidbare Ausnahme.

Manche Charts trennen Pod- und Container-`securityContext` in zwei ähnlich benannte, aber
unterschiedlich verschachtelte Felder. `allowPrivilegeEscalation`/`capabilities` gehören
ausschließlich in den Container-Block. Vor jedem Override im Upstream-Template nachsehen, welches
Feld wohin gemappt wird, und mit `helm template` Pod **und** Container getrennt prüfen.

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
`platform.yaml`), die wiederkehrenden Bausteine (API-Server-Egress, Traefik-/API-Server-Peers,
CNPG-Cluster) in `_helpers.tpl`. Die Prometheus-Scrape-Regeln (`np.ingress.prometheus`) rendern
nichts, solange `namespaces.monitoring` nicht gesetzt ist — es gibt derzeit keinen Prometheus. Namespaces, Kafka-/
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

**Bewusste Grobheiten**, jeweils mit Begründung im Template: `longhorn-system` erlaubt Verkehr innerhalb des Namespaces pauschal (Engine↔Replika über
dynamisch vergebene Ports); Internet-Egress gibt es nur portbeschränkt und nur für cert-manager
(ACME 443, DNS 53), CoreDNS (Upstream 53), ArgoCD-Repo-Server (22/443) und den
Schema-Registrierungs-Job (`apk add`, 80/443). Longhorns Upgrade-Checker ist deshalb abgeschaltet
(`charts/longhorn`).

**Wie die Verkehrsmatrix ermittelt und geprüft wurde.** Aus dem Live-Cluster: Pods/Labels/Ports,
Services, alle Validating-/MutatingWebhookConfigurations und APIServices (= Aufrufe vom
API-Server), alle ServiceMonitor/PodMonitor-Ziele, alle Ingress-Backends. Danach zwei
Simulationen gegen den gerenderten Chart (NetworkPolicy-Semantik inkl. der kube-router-Regel für
lokalen Traffic): (1) jede tatsächlich offene TCP-Verbindung, per `ss` in jedem Pod-Netns und auf
jedem Host über eine Minute mitgeschnitten — 89 Flow-Typen, 0 blockiert; (2) 118 aus der
Konfiguration abgeleitete Soll-Flows (jeder Scrape, jeder Webhook, jedes Ingress-Backend, DNS
für jeden Pod) — 0 blockiert. Gegenprobe mit nur default-deny: 70 der 89 Flows blockiert, die
Simulation erkennt Sperren also. UDP wurde nicht mitgeschnitten (einziger UDP-Verkehr: DNS,
explizit geregelt). Die Messung stammt vom 29.09., also noch mit Observability-Stack.

**Einmalige Schritte beim ersten Rollout auf einem bestehenden Cluster:** Strimzi räumt seine
bisher generierten Policies nicht selbst ab — danach
`kubectl -n kafka delete networkpolicy ticketflow-kafka-network-policy-kafka ticketflow-kafka-entity-operator ticketflow-kafka-kafka-exporter`
(sonst bleiben 9092/9093 über diese alten Policies weiter offen). Die übrigen alten,
chart-eigenen Policies entfernt ArgoCD per `prune` selbst.

**Neue Verbindung, neuer Service:** Regel hier ergänzen, sonst wird der Verkehr verworfen — ohne
Fehlermeldung, nur als Timeout. **Notausgang**, falls ArgoCD sich selbst ausgesperrt hat:
`kubectl -n argocd delete networkpolicy -l app.kubernetes.io/name=network-policies`.
