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
    ├── longhorn.yaml           # verteilter Block-Storage (Helm-Chart, upstream)
    ├── cert-manager.yaml       # cert-manager-Operator (Helm-Chart, upstream)
    ├── cert-manager-issuer.yaml # ClusterIssuer, unser eigener Chart ../charts/cert-manager-config
    ├── external-dns.yaml       # external-dns (Helm-Chart, upstream)
    ├── cnpg-operator.yaml      # CloudNativePG-Operator (Helm-Chart, upstream) — TODO
    ├── strimzi-operator.yaml   # Strimzi-Kafka-Operator (Helm-Chart, upstream) — TODO
    ├── keda.yaml               # KEDA (Helm-Chart, upstream) — TODO
    ├── kube-prometheus-stack.yaml # Prometheus/Grafana/Alertmanager (Helm-Chart, upstream) — TODO
    └── ticketflow.yaml         # unser eigener Chart aus ../charts/ticketflow — TODO
```

Alle Applications nutzen aktuell `project: default` — eine dedizierte `ticketflow-project.yaml`
(Repo-/Namespace-/Ressourcen-Scoping) ist noch offen.

`longhorn`/`cert-manager`/`external-dns` sind **Cluster-Addons, portiert aus
[pfisterer/k3s-dhbw-cloud-role](https://github.com/pfisterer/k3s-dhbw-cloud-role)** — ursprünglich
per Ansible-Rolle installiert (siehe `infra/ansible/README.md` ## Übernommene Funktionen aus
k3s-dhbw-cloud-role, Branch `ansible-bootstrap`, Commit `938a66f`), jetzt stattdessen hier über
ArgoCD/Helm, um die in `CLAUDE.md` ## Infra layering dokumentierte Grenze einzuhalten ("Ansible
macht nichts über k3s/Labels/ArgoCD hinaus"). Die Ansible-Addon-Rollen auf `ansible-bootstrap`
werden dadurch nicht mehr gebraucht und sollten nicht in `main` gemerged werden.

Für `longhorn` und `cert-manager` (Operator) zeigt die `Application` direkt auf den
Upstream-Helm-Repo, Werte inline über `helm.valuesObject` — kein lokaler Chart nötig (siehe
`charts/README.md`: ein Operator ist Cluster-Infrastruktur, keine Anwendungs-Konfiguration).
`cert-manager-issuer.yaml` ist die eine Ausnahme: der `ClusterIssuer` ist unsere **eigene**
Custom Resource (kein Operator), braucht aber Templating (dns01/http01-Switch, optionales
Wildcard-Zertifikat) — dafür liegt in `../charts/cert-manager-config/` ein winziger lokaler
Chart, analog zur CNPG-`Cluster`-CR in `charts/ticketflow`.

## DNS/ACME (cert-manager-issuer, external-dns)

Nutzen die echte DHBW-DNS-Zone `cas438007-at-cas-dhbw-de.users.dhbw.site`
(self-service.dhbw.cloud) — Zone/TSIG-Keyname stehen in `charts/cert-manager-config/values.yaml`
bzw. `gitops/apps/external-dns.yaml` (dieselben Werte an beiden Stellen). Beide `Application`s
sind wie alle anderen auf `syncPolicy.automated` konfiguriert.

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

`TODO`: `cnpg-operator.yaml`, `strimzi-operator.yaml`, `keda.yaml`, `kube-prometheus-stack.yaml`,
`ticketflow.yaml` und `projects/ticketflow-project.yaml` fehlen noch — folgen, sobald die
jeweiligen Chart-Versionen/CRDs feststehen bzw. `charts/ticketflow` Templates hat.
