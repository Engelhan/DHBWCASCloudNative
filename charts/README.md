# charts/ – Helm-Packaging (W1)

Enthält den eigenen Umbrella-Chart [`ticketflow/`](ticketflow/), der `catalog-api` und
`reservation-worker` (beide noch nicht implementiert) bündeln soll, dazu die
CNPG-`Cluster`-Ressource und die Strimzi-`Kafka`-Ressource sowie zugehörige
ConfigMaps/Secrets/ServiceMonitors/HPA-KEDA-Objekte. `notification-worker` ist bewusst **kein**
Teil davon — es hat einen eigenen, standalone Chart, siehe unten.

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
Pod-Label-Selector, siehe `notification-worker/templates/podmonitor.yaml`. Er hat außerdem als einziger
Chart im Repo bislang ein eigenes `values.schema.json` (sonst ist ein Schema an das noch nicht
begonnene `charts/ticketflow` gebunden, siehe root-README `## Entscheidungen` zu W1) sowie
`replicaCount: 3` + `PodDisruptionBudget` als Default. Noch **kein**
`gitops/apps/notification-worker.yaml` — der Chart ist bisher nur lint-/template-fähig, nicht per
ArgoCD deployt.

## Addon-Charts

Alle Cluster-weiten Operatoren/Addons (Longhorn, cert-manager, external-dns, CNPG-Operator,
Strimzi-Operator, kube-prometheus-stack, Loki, Tempo; künftig KEDA) liegen als **eigene, kleine
Wrapper-Charts** hier im Ordner, jeweils mit einer eigenen ArgoCD-`Application` in
[`../gitops/apps/`](../gitops/apps/), die auf `charts/<name>` in diesem Repo zeigt (nicht direkt
auf den Upstream-Helm-Repo) – ein einheitliches Muster für alle Charts in diesem Repo statt zwei
verschiedener (Application-inline-Values vs. lokaler Chart), und alle Werte landen sichtbar
versioniert in einer eigenen `values.yaml` statt in der Application eingebettet.

Zwei Varianten:

- **Wrapper um einen Upstream-Chart** ([`longhorn/`](longhorn/), [`cert-manager/`](cert-manager/),
  [`external-dns/`](external-dns/), [`cnpg-operator/`](cnpg-operator/),
  [`strimzi-operator/`](strimzi-operator/), [`kube-prometheus-stack/`](kube-prometheus-stack/),
  [`loki/`](loki/), [`tempo/`](tempo/)): `Chart.yaml` deklariert den Upstream-Chart als
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
  CNPG-`Cluster`-CR in `ticketflow/`, nur eben nicht Teil des ticketflow-Charts, weil sie zum
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
`charts/ticketflow` existiert; eine zu früh verengte Policy würde sonst später stillschweigend
Metrics-/Log-/Trace-Sammlung blockieren, ohne dass ein Fehler sichtbar wird.
