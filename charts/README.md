# charts/ – Helm-Packaging (W1)

Enthält den eigenen Umbrella-Chart [`ticketflow/`](ticketflow/), der die gesamte Anwendungsebene
bündelt: die drei eigenen Services, die CNPG-`Cluster`-Ressource und die Strimzi-`Kafka`-Ressource
sowie zugehörige ConfigMaps/Secrets/ServiceMonitors/HPA-KEDA-Objekte.

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
