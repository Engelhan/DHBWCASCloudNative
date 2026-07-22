# gitops/ – ArgoCD (W2)

Ab dem Punkt, an dem Ansible ArgoCD auf dem frischen k3s-Cluster installiert hat
(`infra/ansible/roles/argocd_bootstrap/`), übernimmt ArgoCD per **App-of-Apps-Pattern** den
gesamten weiteren Rollout aus diesem Ordner.

## Struktur

```
gitops/
├── bootstrap/
│   └── argocd-install.yaml     # Helm-Values für die ArgoCD-Installation selbst
├── projects/
│   └── ticketflow-project.yaml # ArgoCD AppProject: erlaubte Repos/Namespaces/Ressourcen (TODO)
└── apps/
    ├── app-of-apps.yaml        # Root-Application, zeigt (nicht-rekursiv) auf diesen Ordner
    ├── longhorn.yaml           # verteilter Block-Storage, Wrapper-Chart ../charts/longhorn
    ├── cert-manager.yaml       # cert-manager-Operator, Wrapper-Chart ../charts/cert-manager
    ├── cert-manager-issuer.yaml # ClusterIssuer, eigener Chart ../charts/cert-manager-config
    ├── external-dns.yaml       # external-dns, Wrapper-Chart ../charts/external-dns
    ├── argocd-ingress.yaml     # oeffentlicher ArgoCD-Zugriff, eigener Chart ../charts/argocd-ingress
    ├── cnpg-operator.yaml      # CloudNativePG-Operator, Wrapper-Chart ../charts/cnpg-operator
    ├── strimzi-operator.yaml   # Strimzi-Kafka-Operator, Wrapper-Chart ../charts/strimzi-operator
    ├── kube-prometheus-stack.yaml # Prometheus/Alertmanager/Grafana, Wrapper-Chart ../charts/kube-prometheus-stack
    ├── loki.yaml               # Log-Aggregation, Wrapper-Chart ../charts/loki
    ├── tempo.yaml              # Trace-Aggregation, Wrapper-Chart ../charts/tempo
    ├── keda.yaml               # KEDA, Wrapper-Chart ../charts/keda — TODO
    └── ticketflow.yaml         # unser eigener Chart aus ../charts/ticketflow — TODO
```

Jede `Application` in diesem Ordner zeigt auf einen Chart unter [`../charts/`](../charts/) in
diesem Repo — nie direkt auf einen Upstream-Helm-Repo. Für Addons, die es upstream als Chart
gibt (Longhorn, cert-manager, external-dns, CNPG, Strimzi, kube-prometheus-stack, Loki, Tempo,
künftig KEDA), ist das ein **Wrapper-Chart** (`Chart.yaml` mit `dependencies` auf den
Upstream-Chart, eigene `values.yaml`) statt Application-inline-`helm.valuesObject` — ein
einheitliches Muster für alle Charts in diesem Repo, siehe `charts/README.md`.

`cnpg-operator`/`strimzi-operator`/`kube-prometheus-stack` brauchen zusätzlich
`syncOptions: [ServerSideApply=true]`: alle drei Upstream-Charts bündeln einzelne CRDs deutlich
über 262144 Byte (CNPGs `poolers`-CRD über 650 KB, Strimzis `Kafka`-CRD über 330 KB,
kube-prometheus-stacks `Prometheus`-CRD über 800 KB) — Client-Side-Apply würde die komplette
Ressource zusätzlich als `last-applied-configuration`-Annotation speichern und damit das harte
Annotation-Größenlimit der Kubernetes-API sprengen ("Too long"). Exakt derselbe Fehler wie beim
ArgoCD-Install selbst, siehe `CLAUDE.md` ## Infra layering (Ansible specifics) —
Server-Side-Apply schreibt diese Annotation gar nicht erst. Loki/Tempo bringen selbst keine CRDs
mit, brauchen das nicht.

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

**`app-of-apps.yaml` schließt sich per `source.directory.exclude` selbst aus** der Ressourcenliste
aus, die es synct — es liegt im selben Ordner, den es (nicht-rekursiv) einliest. Ohne das
`exclude` würde die Application sich selbst als verwaltete Ressource auffassen; mit
`prune: true` + `selfHeal: true` hätte ein Sync dann versuchen können, die eigene
`Application`-Ressource zu löschen (und über deren `resources-finalizer` kaskadierend alle
Kind-Applications mit ihr) — auf einem laufenden Cluster beobachtet, bevor der Fix live war.
Jede neue Datei in `apps/` ist automatisch sicher (sie referenziert sich nicht selbst); nur
`app-of-apps.yaml` selbst braucht dieses `exclude`.

## ArgoCD-UI öffentlich erreichbar (`argocd-ingress`)

`gitops/apps/argocd-ingress.yaml` + `charts/argocd-ingress/` machen die ArgoCD-UI über die echte
DHBW-DNS-Zone erreichbar (`argocd.cas438007-at-cas-dhbw-de.users.dhbw.site` per Default, siehe
`charts/argocd-ingress/values.yaml`). Statt TLS an Traefik zu terminieren (bräuchte
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
kein DNS-Record, live so beobachtet. `charts/argocd-ingress/templates/ingressroute-tcp.yaml`
setzt diese Annotation deshalb explizit auf die aktuellen IPv6-VIPs des
Traefik-`LoadBalancer`-Service (`charts/argocd-ingress/values.yaml` → `externalDnsTargets`,
IPv4-VIPs bewusst weggelassen, analog zu `--exclude-target-net=10.0.0.0/8` bei den
Ingress-basierten Records). Das ist ein statischer Wert, kein dynamisch nachgeführter — **nach
einem Cluster-Rebuild (neue VIPs) muss `externalDnsTargets` von Hand aktualisiert werden**:
`kubectl -n kube-system get svc traefik -o jsonpath='{.status.loadBalancer.ingress[*].ip}'`.

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

**Bei einem eigenen Fork** steht `git@github.com:Engelhan/DHBWCASCloudNative.git` in sechs
Dateien (allen `gitops/apps/*.yaml`) — nicht einzeln suchen und ersetzen, sondern
`../deploy.env.example` nach `../deploy.env` kopieren, eigene Fork-URL eintragen und
`python3 ../scripts/personalize.py` laufen lassen, siehe Root-`README.md` ## Betrieb.

`longhorn`/`cert-manager`/`external-dns` sind **Cluster-Addons, portiert aus
[pfisterer/k3s-dhbw-cloud-role](https://github.com/pfisterer/k3s-dhbw-cloud-role)** — ursprünglich
per Ansible-Rolle installiert (siehe `infra/ansible/README.md` ## Übernommene Funktionen aus
k3s-dhbw-cloud-role, Branch `ansible-bootstrap`, Commit `938a66f`), jetzt stattdessen hier über
ArgoCD/Helm, um die in `CLAUDE.md` ## Infra layering dokumentierte Grenze einzuhalten ("Ansible
macht nichts über k3s/Labels/ArgoCD hinaus"). Die Ansible-Addon-Rollen auf `ansible-bootstrap`
werden dadurch nicht mehr gebraucht und sollten nicht in `main` gemerged werden.

## DNS/ACME (cert-manager-issuer, external-dns)

Nutzen die echte DHBW-DNS-Zone `cas438007-at-cas-dhbw-de.users.dhbw.site`
(self-service.dhbw.cloud) — Zone/TSIG-Keyname stehen in `charts/cert-manager-config/values.yaml`
bzw. `charts/external-dns/values.yaml` (dieselben Werte an beiden Stellen — bei einem eigenen
Fork per `../scripts/personalize.py` an beiden Stellen gleichzeitig ersetzbar, siehe ## Privates
Repo oben). Beide `Application`s sind wie alle anderen auf `syncPolicy.automated` konfiguriert.

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
kubectl apply -f gitops/apps/app-of-apps.yaml   # normalerweise automatisch via argocd_bootstrap
argocd app list
argocd app get longhorn
argocd app get cert-manager
```

`TODO`: `keda.yaml` (mit eigenem Wrapper-Chart unter `../charts/`, analog zu allen anderen
Addons), `ticketflow.yaml` und `projects/ticketflow-project.yaml` fehlen noch — folgen,
sobald die jeweiligen Chart-Versionen/CRDs feststehen bzw. `charts/ticketflow` Templates hat.
