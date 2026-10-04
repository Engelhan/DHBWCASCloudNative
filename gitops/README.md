# gitops/ – ArgoCD (W2)

Ab dem Punkt, an dem Ansible ArgoCD auf dem frischen k3s-Cluster installiert hat
(`infra/ansible/roles/argocd_bootstrap/`), übernimmt ArgoCD den gesamten weiteren Rollout aus
diesem Ordner — gesteuert über **zwei `ApplicationSet`s, die nacheinander ausgerollt werden**
(siehe ## Gestufter Rollout).

## Struktur

```
gitops/
├── env/
│   └── cluster.yaml         # ZENTRALE Konfiguration: Fork-URL, DNS-Zone, ACME, Images, dev/prod
└── applicationset.yaml      # zwei ApplicationSets (platform, apps) -> zusammen 21 Applications
```

`env/cluster.yaml` ist die einzige git-getrackte Datei, die für ein eigenes Deployment angepasst
werden muss. Sie wird an vier Stellen gelesen:

| Leser | wofür |
|---|---|
| ArgoCD (git-files-Generator) | Repo-URL, Revision, Environment der ApplicationSets |
| ArgoCD (Multi-Source `$values`) | reicht die Datei jedem Chart als zusätzliche Values-Datei durch |
| Helm-Charts | `.Values.ticketflow.*` — DNS-Zone, Image-Registry/-Tag, ACME-Kontakt, TSIG-Key-Name |
| Terraform / Ansible | `namePrefix` bzw. `repoURL` (siehe `infra/`) |

### Warum ein ApplicationSet statt 19 Application-Dateien

Vorher lag unter `apps/` je eine `Application`-Datei pro Komponente plus eine
`app-of-apps.yaml` als Root. Die Repo-URL stand damit **19-mal** im Repo (plus einmal in
`infra/ansible/roles/gitops_secrets/`) — für einen Fork waren das 20 Stellen, von denen eine
übersehene genügte, damit ArgoCD still auf das Original-Repo zeigte. Jetzt steht sie nur noch in
den `repoURL`s der beiden git-Generatoren in `applicationset.yaml` (eine je ApplicationSet, in
derselben Datei). Dass sie dort wörtlich stehen bleiben muss, ist ein Henne-Ei-Problem — der
Generator muss wissen, aus welchem Repo er `env/cluster.yaml` überhaupt lesen soll.

Jedes Listen-Element beschreibt eine Application mit `name`, `chartPath`, `namespace` und
optional `serverSideApply`, `ignoreReplicas`. Alles, was nicht für alle Applications
gleich ist, steht im `templatePatch` am Ende der Datei — `spec.template` ist ein typisiertes
Kubernetes-Objekt, in dem Go-Ausdrücke nur innerhalb von String-Werten funktionieren, nicht als
`{{- if }}`-Blöcke um YAML-Schlüssel herum.

Jede erzeugte `Application` zeigt auf einen Chart unter [`../charts/`](../charts/) in diesem Repo
— nie direkt auf ein Upstream-Helm-Repo. Für Addons, die es upstream als Chart gibt (Longhorn,
cert-manager, external-dns, CNPG, Strimzi, kube-prometheus-stack, Loki, Tempo, KEDA, AKHQ), ist
das ein **Wrapper-Chart** (`Chart.yaml` mit `dependencies` auf den Upstream-Chart, eigene
`values.yaml`), siehe `charts/README.md`.

### dev/prod

Jeder Chart bekommt zusätzlich `values-<ticketflow.environment>.yaml` als Values-Datei, per
`ignoreMissingValueFiles: true` übersprungen, wo es die Datei nicht gibt. Ein Umschalten von
`dev` auf `prod` ist deshalb genau eine Zeile in `env/cluster.yaml` und braucht keine Änderung am
ApplicationSet — auch dann nicht, wenn ein Chart später einen dev/prod-Split bekommt.

`cnpg-operator`/`strimzi-operator`/`kube-prometheus-stack`/`keda`/`external-secrets-operator`
brauchen zusätzlich `syncOptions: [ServerSideApply=true]`: alle fünf Upstream-Charts bündeln
einzelne CRDs deutlich über bzw. (bei `external-secrets-operator`) knapp unter 262144 Byte (CNPGs
`poolers`-CRD über 650 KB, Strimzis `Kafka`-CRD über 330 KB, kube-prometheus-stacks
`Prometheus`-CRD über 800 KB, `external-secrets-operator`s `secretstores`/`clustersecretstores`
je ~252 KB reines YAML, gemessen) — Client-Side-Apply würde die komplette
Ressource zusätzlich als `last-applied-configuration`-Annotation speichern und damit das harte
Annotation-Größenlimit der Kubernetes-API sprengen ("Too long"). Exakt derselbe Fehler wie beim
ArgoCD-Install selbst, siehe `CLAUDE.md` ## Infra layering (Ansible specifics) —
Server-Side-Apply schreibt diese Annotation gar nicht erst. Loki/Tempo/`openbao`/
`openbao-postgres`/`openbao-secretstore` bringen selbst keine CRDs mit, brauchen das nicht.

## Observability-Stack (kube-prometheus-stack, Loki, Tempo)

Voller LGTM-Stack (Logs/Grafana/Tempo/Metrics) in **einem** Grafana, alle drei Applications im
Namespace `monitoring`, Persistenz durchgehend über Longhorn (bereits Default-StorageClass):

- **kube-prometheus-stack**: Prometheus (7 Tage Retention, 20Gi), Alertmanager (2Gi), Grafana
  (5Gi, Service `kube-prometheus-stack-grafana`) sowie die Prometheus-Operator-CRDs
  (ServiceMonitor/PodMonitor/PrometheusRule), die Longhorn/CNPG/Strimzi/Loki/Tempo für eigenes
  Metrics-Scraping brauchen.
- **Loki**: `SingleBinary`-Deployment-Modus (keine getrennten Read-/Write-/Backend-Komponenten -
  reicht für die Lab-Größe) mit eingebautem MinIO als S3-kompatiblem Objektspeicher (offiziell
  empfohlener Weg ohne echtes Cloud-Object-Storage), beides auf je einer Longhorn-PVC. Standard-
  Entrypoint ist der Gateway-Service `loki-gateway`.
- **Tempo**: einfacher Single-Instance-Chart, lokales Storage-Backend auf einer Longhorn-PVC,
  Service `tempo` auf Port 3200.
- Grafana bekommt Loki (`uid: loki`) und Tempo (`uid: tempo`) als `additionalDataSources` (siehe
  `charts/kube-prometheus-stack/values.yaml`) — inklusive Traces→Logs-Verknüpfung
  (`tracesToLogsV2`) und Service-Graph aus Tempo-Spans gegen Prometheus (`serviceMap`).
- Kein Sync-Wave zwischen den drei Applications: Lokis/Tempos `ServiceMonitor`-Ressourcen
  brauchen die von kube-prometheus-stack bereitgestellte CRD, aber falls die beim ersten Sync
  noch fehlt, holt `selfHeal` das im nächsten Reconcile automatisch nach (dieselbe lose Kopplung
  wie zwischen `cert-manager-issuer` und `argocd-ingress`).

Alle Applications nutzen aktuell `project: default` — eine dedizierte `ticketflow-project.yaml`
(Repo-/Namespace-/Ressourcen-Scoping) ist noch offen.

### Gestufter Rollout

Die 21 Applications verteilen sich auf zwei ApplicationSets mit dem Label `ticketflow.io/stage`:

| Stufe | ApplicationSet | Inhalt |
|---|---|---|
| 1 | `ticketflow-platform` | Longhorn, cert-manager (+ ClusterIssuer), external-dns, Traefik-Config, ArgoCD-Ingress, CNPG-/Strimzi-Operator, KEDA, External Secrets, OpenBao (+ eigene Postgres-DB, ClusterSecretStore) |
| 2 | `ticketflow-apps` | NetworkPolicies, App-Postgres, Kafka, Apicurio, AKHQ, catalog-api/frontend, reservation-/notification-worker |

`infra/ansible/roles/gitops_rollout` wendet Stufe 2 erst an, wenn **jede** Application aus Stufe 1
`Synced` und `Healthy` ist. `Healthy` bedeutet bei Deployments/StatefulSets, dass alle Pods der
aktuellen Revision ihre Readiness-Probe bestehen; bei Operator-CRs die Ready-Condition (ArgoCD
bringt Health-Checks für CNPG `Cluster`, cert-manager `ClusterIssuer`, ESO `ClusterSecretStore`,
Strimzi `Kafka` u. a. mit); bei `openbao` zusätzlich den durchgelaufenen Bootstrap-Hook. Damit
startet die Anwendung erst, wenn StorageClass, CRDs, Zertifikats-Issuer und ein entsiegelter
Secret-Vault tatsächlich bereitstehen — vorher hingen ExternalSecrets, PushSecrets, KafkaUser
und ScaledObjects in Fehler-/Retry-Schleifen, bis die Plattform zufällig nachgezogen hatte.

Warum so und nicht anders:
- **ArgoCD-Sync-Waves** wirken nur innerhalb einer Application bzw. auf Applications unter einer
  App-of-Apps. Auf von einem ApplicationSet erzeugte Applications haben sie keine Wirkung — die
  früheren `syncWave`-Felder hier waren wirkungslos und sind entfernt.
- **ApplicationSet-Progressive-Syncs (`RollingSync`)** würden die Reihenfolge auch bei jedem Push
  einhalten, schalten aber Auto-Sync und `selfHeal` aller erzeugten Applications ab (im
  ArgoCD-3.4.5-Code geprüft: eine reine Live-Drift löst dort keinen Sync aus, nur eine neue
  Revision). Das widerspricht W2.

Innerhalb einer Stufe hängen einzelne Apps weiterhin voneinander ab (z. B. `cert-manager-issuer`
von den cert-manager-CRDs). Deshalb tragen **alle** Applications `retry` mit Backoff (30 s bis
5 min) — ohne das versucht ArgoCD einen gescheiterten automatischen Sync für denselben Commit nie
erneut, und die Stufe würde nie grün. `kafka-cluster` und `apicurio` müssen in derselben Stufe
bleiben: der Schema-Registrierungs-Hook von `kafka-cluster` wartet ohne Timeout auf Apicurio,
Apicurio braucht Kafka.

Die Stufung gilt für das **Anwenden der ApplicationSets** (erster Rollout, erneuter
`bootstrap-gitops.yml`-Lauf). Danach synct ArgoCD jede Application bei jedem Push unabhängig — die
Plattform existiert dann bereits. Bleibt eine Plattform-App rot, bricht Ansible nach 30 Minuten
mit der Liste der noch nicht grünen Applications ab, statt die Anwendung auf eine halbe Plattform
zu setzen.

**Der Root ist kein `Application` mehr, sondern die beiden `ApplicationSet`s selbst.** Damit entfällt auch
der frühere `source.directory.exclude`-Kunstgriff: die alte `app-of-apps.yaml` lag im selben
Ordner, den sie synchronisierte, und hätte sich mit `prune: true` + `selfHeal: true` sonst selbst
löschen können (über ihren `resources-finalizer` kaskadierend alle Kind-Applications mit ihr — auf
einem laufenden Cluster beobachtet, bevor der Fix live war). Ein ApplicationSet verwaltet seine
Applications, ohne sich selbst als Ressource zu sehen.

**Migration von App-of-Apps auf das ApplicationSet** (einmalig, auf einem bereits laufenden
Cluster): die alte Root-Application muss ohne Kaskade verschwinden, sonst reißt ihr Finalizer alle
Workloads mit. Das ApplicationSet übernimmt die verwaisten Applications danach über ihre Namen.

```bash
kubectl -n argocd patch app app-of-apps --type=merge -p '{"metadata":{"finalizers":null}}'
kubectl -n argocd delete app app-of-apps --cascade=orphan
kubectl apply -f gitops/applicationset.yaml
kubectl -n argocd get applications      # 19 Stueck, keine Duplikate
```

**Migration von einem ApplicationSet (`ticketflow`) auf die zwei gestuften** (einmalig, auf einem
laufenden Cluster): das alte Set muss ohne Kaskade verschwinden — sonst löscht die
Garbage-Collection über die ownerReferences alle 21 Applications samt Workloads. `--cascade=orphan`
entfernt nur die ownerReferences; die neuen Sets übernehmen die Applications danach über ihre
Namen.

```bash
kubectl -n argocd delete applicationset ticketflow --cascade=orphan
cd infra/ansible && ansible-playbook playbooks/bootstrap-gitops.yml   # Stufe 1, warten, Stufe 2
kubectl -n argocd get applications -L ticketflow.io/stage             # 21 Stueck, je mit Stufe
```

## ArgoCD-UI öffentlich erreichbar (`argocd-ingress`)

Das Element `argocd-ingress` im ApplicationSet + `charts/argocd-ingress/` machen die ArgoCD-UI über die echte
DHBW-DNS-Zone erreichbar (`argocd.<ticketflow.dnsZone>`, abgeleitet aus `env/cluster.yaml`). Statt TLS an Traefik zu terminieren (bräuchte
`server.insecure=true` + ConfigMap-Patch + Neustart von `argocd-server`) nutzt der Chart
TLS-Passthrough: ein `Certificate` erzeugt das Secret `argocd-server-tls`, das ArgoCDs
API-Server automatisch selbst für sein TLS verwendet (offiziell dokumentiertes Verhalten), eine
Traefik `IngressRouteTCP` mit `tls.passthrough: true` routet nur anhand der SNI zum
`argocd-server`-Service durch, ohne den Traffic zu entschlüsseln.

**`external-dns` veröffentlicht hier KEINEN Record automatisch** — anders als bei normalen
`Ingress`-Objekten liest external-dns' `traefik-proxy`-Source für `IngressRoute`/
`IngressRouteTCP`/`-UDP` das Ziel niemals aus dem Status des Traefik-`LoadBalancer`-Service,
sondern ausschließlich aus der Annotation `external-dns.alpha.kubernetes.io/target`
(`source/traefik_proxy.go` im external-dns-Repo) — ohne sie: "No endpoints could be generated",
kein DNS-Record, live so beobachtet. Statt hier rohe Traefik-VIPs zu hinterlegen (die bei jedem
Cluster-Rebuild neu vergeben werden und damit gegen "ein Ablauf bringt alles hoch" verstoßen
würden), setzt `charts/argocd-ingress/templates/ingressroute-tcp.yaml` diese Annotation auf einen
stabilen Hostnamen (`charts/argocd-ingress/values.yaml` → `externalDnsTarget`,
`traefik.<zone>`). Dieser Hostname wiederum wird vom Element `traefik-config` +
`charts/traefik-config/` erzeugt: ein `HelmChartConfig`-Objekt (`helm.cattle.io/v1`, Name/
Namespace `traefik`/`kube-system`) mergt eine `external-dns.alpha.kubernetes.io/hostname`-
Annotation in k3s' eigenes, gebündeltes Traefik-`HelmChart` — der offiziell dokumentierte Weg,
zusätzliche Werte für ein von k3s selbst verwaltetes Chart zu setzen, ohne mit dessen
`helm-controller` um die Ownership zu konkurrieren. Für den Traefik-`Service` selbst (anders als
für `IngressRoute`/`-TCP`/`-UDP`) kann external-dns' `service`-Source das Ziel automatisch aus dem
`LoadBalancer`-Status ableiten, sodass `traefik.<zone>` bei jedem VIP-Wechsel automatisch
mitzieht — `argocd.<zone>` zeigt dann effektiv per CNAME auf `traefik.<zone>`. Kein manueller
Schritt nach einem Cluster-Rebuild mehr nötig.

## Privates Repo — ArgoCD braucht ein Credential

Dieses Repo ist **nicht öffentlich**. Da jede `Application` auf einen Chart in *diesem* Repo
zeigt (siehe oben), kann ArgoCDs repo-server ohne registriertes Credential gar nichts klonen —
alle Applications blieben dauerhaft in `Unknown`/`ErrorSyncStatus` hängen. Deshalb nutzen alle
`repoURL: git@github.com:...` (SSH), nicht `https://...`, und
`infra/ansible/roles/gitops_secrets` legt dafür einen ArgoCD-Repository-Credential-Secret
(`repo-ticketflow`, Namespace `argocd`) aus einem lokalen, git-ignorierten SSH-Deploy-Key an —
siehe `infra/ansible/README.md` ## GitOps-Secrets für die Erzeugung/Aktivierung. Die
Upstream-Helm-Repos selbst (`charts.longhorn.io`, `charts.jetstack.io`,
`kubernetes-sigs.github.io`) sind weiterhin öffentlich — dafür braucht nur der jeweilige
Wrapper-Chart in `Chart.yaml.dependencies` kein Credential, das baut ArgoCD beim Sync per
`helm dependency build` automatisch.

**Bei einem eigenen Fork** wird die Fork-URL an genau zwei Stellen eingetragen:
`env/cluster.yaml` → `ticketflow.repoURL` (daraus speisen sich alle erzeugten Applications und
das Credential aus `infra/ansible/roles/gitops_secrets`) und die `repoURL`s der beiden
git-Generatoren in `applicationset.yaml` (Henne-Ei, siehe ## Struktur). Ein Suchen-und-Ersetzen-Skript dafür gibt es
nicht mehr — es wurde zusammen mit `gitops/apps/` entfernt, weil es nichts mehr zu verteilen gibt.

`longhorn`/`cert-manager`/`external-dns` sind **Cluster-Addons, portiert aus
[pfisterer/k3s-dhbw-cloud-role](https://github.com/pfisterer/k3s-dhbw-cloud-role)** — ursprünglich
per Ansible-Rolle installiert (siehe `infra/ansible/README.md` ## Übernommene Funktionen aus
k3s-dhbw-cloud-role, Branch `ansible-bootstrap`, Commit `938a66f`), jetzt stattdessen hier über
ArgoCD/Helm, um die in `CLAUDE.md` ## Infra layering dokumentierte Grenze einzuhalten ("Ansible
macht nichts über k3s/Labels/ArgoCD hinaus"). Die Ansible-Addon-Rollen auf `ansible-bootstrap`
werden dadurch nicht mehr gebraucht und sollten nicht in `main` gemerged werden.

## DNS/ACME (cert-manager-issuer, external-dns)

Nutzen die echte DHBW-DNS-Zone aus `env/cluster.yaml` (`ticketflow.dnsZone`, registriert über
self-service.dhbw.cloud). Zone, RFC2136-Server und TSIG-Key-Name stehen dort **einmal** und
werden von beiden Charts gelesen — vorher lagen dieselben Werte doppelt in
`charts/cert-manager-config/values.yaml` und `charts/external-dns/values.yaml` und konnten
auseinanderlaufen.

`charts/cert-manager-config` liest sie direkt in seinen eigenen Templates. `charts/external-dns`
kann das nicht, weil die zonenabhängigen Werte in den Values des **gewrappten** Upstream-Charts
stehen und Helm Values-Dateien nicht templatet: dort rendert `templates/zone-configmap.yaml` eine
ConfigMap, die per `$(DNS_ZONE)`/`$(RFC2136_ZONE)`/`$(RFC2136_KEYNAME)` in den Container-Args
expandiert wird — derselbe Mechanismus, den der Chart für
`--rfc2136-tsig-secret=$(RFC2136_TSIG_SECRET)` ohnehin schon nutzt.

Beide `Application`s sind wie alle anderen auf `syncPolicy.automated` konfiguriert.

**Das eigentliche TSIG-Secret steht bewusst NICHT im Repo** (nur die Referenz
`external-dns-rfc2136-tsig` / `cert-manager-rfc2136-tsig`) — vor dem ersten Sync manuell anlegen:

```bash
kubectl -n cert-manager create secret generic cert-manager-rfc2136-tsig \
  --from-literal=secret=<TSIG-Wert>
kubectl create namespace external-dns
kubectl -n external-dns create secret generic external-dns-rfc2136-tsig \
  --from-literal=secret=<TSIG-Wert>
```

Ohne dieses Secret bleiben die jeweiligen Pods in `CreateContainerConfigError` hängen, bis das
Secret existiert — `selfHeal` holt den Rollout dann automatisch nach, kein erneuter Sync nötig.

Jede `Application` in diesem Ordner ist auf `syncPolicy.automated.selfHeal: true` und
`prune: true` konfiguriert: manuelle `kubectl edit`-Änderungen (z.B. eine per Hand hochgesetzte
Replica-Zahl) werden von ArgoCD automatisch wieder auf den in Git definierten Zustand
zurückgedreht – das ist der im Screencast zu zeigende Drift-Nachweis für W2.

## Warum ArgoCD statt reinem `kubectl apply`/CI-Pipeline

Eine CI-Pipeline weiß nur, was sie selbst zuletzt appliziert hat; sie merkt nicht, wenn jemand
danach manuell am Cluster vorbeikonfiguriert. Ein im Cluster laufender GitOps-Agent vergleicht
kontinuierlich Ist- gegen Soll-Zustand aus Git und regelt Abweichungen selbstständig zurück – das
ist der Kern dessen, was W2 fordert ("Drift wird zurückgedreht"), nicht nur eine andere Art zu
deployen.

## Verwendung

```bash
# normalerweise automatisch und gestuft via infra/ansible/roles/gitops_rollout; von Hand:
kubectl apply -f gitops/applicationset.yaml -l ticketflow.io/stage=platform
kubectl apply -f gitops/applicationset.yaml -l ticketflow.io/stage=apps   # erst wenn Stufe 1 gruen
kubectl -n argocd get applications -L ticketflow.io/stage
argocd app list
argocd app get longhorn
argocd app get cert-manager
```

## kafka-cluster / ticketflow-app

Die Elemente `kafka-cluster` und `ticketflow-app` zeigen beide auf
`targetRevision: main`. Beide nutzen `helm.valueFiles: [values-dev.yaml]` — auf dem Lab-Cluster
bewusst die kleine Variante (1 Kafka-Broker, 1 Replica je App-Service) statt `values-prod.yaml`.
Destination-Namespace für beide `ticketflow`, per `CreateNamespace=true` — getrennt vom
Strimzi-Operator-Namespace `kafka` (dort laufen nur Operator/AKHQ/Apicurio als
Plattform-Tooling, nicht die Kafka-Cluster-Instanz selbst).

`charts/kafka-cluster/` (Kafka-CR, KafkaNodePools, KafkaTopics, Apicurio-Schema-Registrierung)
und `charts/ticketflow-app/` (catalog-api + frontend) sind bewusst zwei getrennte Charts/
Applications (Team-Entscheidung: nicht gemeinsam deployen, damit ein Sync/Rollback des einen den
anderen nicht anfasst) — und seit der Namespace-Trennung auch in getrennten Namespaces: die
Kafka-Plattform (Operator, Cluster, Apicurio, AKHQ) in `kafka`, die Anwendung in `ticketflow`.
Beide Namen stehen in `env/cluster.yaml` (`ticketflow.namespaces.app`/`.kafka`); die Clients
leiten ihre Bootstrap-Adresse daraus ab. Einzige Ausnahme sind die `KafkaUser`-CRs, die
`charts/kafka-cluster` in den App-Namespace rendert, weil der Strimzi User Operator das
generierte SCRAM-Secret immer im Namespace der CR ablegt und `secretKeyRef` keine
Namespace-Grenze überschreitet — Details in `charts/kafka-cluster/README.md` ## Namespaces.
`charts/kafka-cluster/` hieß ursprünglich `charts/ticketflow/` (bis 2026-09-16, umbenannt um
Verwechslungen mit `charts/ticketflow-app/` zu vermeiden — reiner `git mv`, keine
Inhaltsänderung). `notification-worker`/CNPG-Cluster sind ebenfalls **nicht** Teil von
`kafka-cluster` oder `ticketflow-app` — eigene, volleigenständige Charts
(`charts/notification-worker/`, `charts/postgres-cluster/`) mit eigenen Applications.

`TODO`: ein dediziertes `AppProject` (Repo-/Namespace-/Ressourcen-Scoping) fehlt noch — alle
Applications laufen bislang mit `project: default`.

## OpenBao / Secret-Vault (B1)

Vier Applications: `openbao-postgres` (OpenBaos eigene CNPG-`Cluster`-CR, Namespace `openbao`),
`openbao` (der eigentliche Secret-Vault, HA über PostgreSQL-Storage-Backend, gleicher Namespace),
`external-secrets-operator` (Namespace `external-secrets`) und `openbao-secretstore` (die eine
`ClusterSecretStore`-CR gegen OpenBao, ebenfalls im Namespace `external-secrets`, weil sie mit dem
Operator zusammengehört). Details zu Chart-Aufbau/Werten stehen in `CLAUDE.md` ## Current
implementation status; hier nur der GitOps-spezifische Teil.

**Alle vier liegen in Stufe 1 (`ticketflow-platform`)**, ihre Konsumenten (`postgres-cluster`,
`kafka-cluster`, `akhq`, `notification-worker`) in Stufe 2 — die Anwendung startet also erst, wenn
der `ClusterSecretStore` gegen ein entsiegeltes OpenBao `Valid` ist (siehe ## Gestufter Rollout).
Innerhalb der Stufe sorgt `retry` für die Reihenfolge. Die tatsächliche Abhängigkeitskette bis alles zum ersten Mal grün ist,
ist trotzdem länger als bei jedem anderen Addon-Paar in diesem Repo: `cnpg-operator` →
`openbao-postgres` → `openbao` (Bootstrap-Hook-Job läuft) → `openbao-secretstore` (wird `Valid`)
→ jede migrierte `ExternalSecret` (`charts/postgres-cluster`, `charts/kafka-cluster`,
`charts/akhq`, `charts/notification-worker`) kann erst danach syncen. ArgoCD zeigt beim ersten
echten Rollout deshalb erwartungsgemäß vorübergehend `OutOfSync`/`Error`/`Degraded` auf mehreren
dieser Applications gleichzeitig, bis jedes Glied per `selfHeal` nachgezogen hat — kein Bug,
sondern derselbe "deklarativ da, funktional wartend"-Fall wie beim `cert-manager-config`
`ClusterIssuer`, der ebenfalls erst nach dem TSIG-Secret funktioniert (siehe ## DNS/ACME oben).

**Seit 2026-09-23 zusätzlich `PushSecret` (K8s → OpenBao, das Gegenstück zu `ExternalSecret`) für
`charts/postgres-cluster` und `charts/kafka-cluster`:** CNPGs und Strimzis selbst generierte
Credentials werden nach OpenBao gespiegelt, damit die Vault wirklich jedes Credential im Cluster
zeigt (zentrale Audit-Fläche, nicht nur eine Namespace-Überbrückung). Für `postgres-cluster`
verbessert das sogar die Abhängigkeitskette: die CNPG-`Cluster`-CR bootstrapt jetzt unabhängig von
OpenBao (kein vorgegebenes `bootstrap.initdb.secret` mehr, CNPG generiert es selbst) — nur der
Rückweg zur konventionell benannten Secret-Kopie, die `catalog-api`/`reservation-worker`
tatsächlich per `secretKeyRef` lesen, hängt noch am `PushSecret`→OpenBao→`ExternalSecret`-Dreieck.
Vorher musste die Datenbank selbst auf einen bereits entsiegelten, befüllten Vault warten, bevor
sie überhaupt initialisieren konnte — das ist mit dieser Umstellung entfallen.

**Bootstrap ist vollständig GitOps-nativ, keine Ansible-Rolle:** `charts/openbao` bringt einen
`post-install,post-upgrade`-Helm-Hook-`Job` mit (`templates/bootstrap-job.yaml`), der bei jedem
Sync automatisch Init/Unseal/KV-Setup übernimmt — analog zum bestehenden
Apicurio-Schema-Registrierungs-Job in `charts/kafka-cluster`. Einzige empfohlene manuelle Aktion
nach dem allerersten erfolgreichen Sync: eine Offline-Sicherung des Unseal-Keys/Root-Tokens außerhalb
des Clusters ziehen (ein Helm-Hook-Job kann anders als eine Ansible-Rolle nicht auf die Maschine
des Operators schreiben):

```bash
kubectl -n openbao get secret openbao-unseal-keys -o yaml > openbao-unseal-keys.backup.yaml
# openbao-unseal-keys.backup.yaml sicher (z.B. verschlüsselt) ablegen, NICHT committen.
```

Ohne dieses Backup ist der Shamir-`1-von-1`-Key (bewusste Laborvereinfachung, siehe Root-README
## Bonus B1) ausschließlich im gleichnamigen K8s-Secret vorhanden — geht dieses verloren, sind die
in OpenBao gespeicherten Werte nicht wiederherstellbar.
