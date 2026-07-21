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
│   └── ticketflow-project.yaml # ArgoCD AppProject: erlaubte Repos/Namespaces/Ressourcen
└── apps/
    ├── app-of-apps.yaml        # Root-Application, zeigt auf diesen Ordner
    ├── cnpg-operator.yaml      # CloudNativePG-Operator (Helm-Chart, upstream)
    ├── strimzi-operator.yaml   # Strimzi-Kafka-Operator (Helm-Chart, upstream)
    ├── keda.yaml               # KEDA (Helm-Chart, upstream)
    ├── kube-prometheus-stack.yaml # Prometheus/Grafana/Alertmanager (Helm-Chart, upstream)
    └── ticketflow.yaml         # unser eigener Chart aus ../charts/ticketflow
```

Jede `Application` ist auf `syncPolicy.automated.selfHeal: true` und `prune: true` konfiguriert:
manuelle `kubectl edit`-Änderungen (z.B. eine per Hand hochgesetzte Replica-Zahl) werden von
ArgoCD automatisch wieder auf den in Git definierten Zustand zurückgedreht – das ist der im
Screencast zu zeigende Drift-Nachweis für W2.

## Warum ArgoCD statt reinem `kubectl apply`/CI-Pipeline

Eine CI-Pipeline weiß nur, was sie selbst zuletzt appliziert hat; sie merkt nicht, wenn jemand
danach manuell am Cluster vorbeikonfiguriert. Ein im Cluster laufender GitOps-Agent vergleicht
kontinuierlich Ist- gegen Soll-Zustand aus Git und regelt Abweichungen selbstständig zurück – das
ist der Kern dessen, was W2 fordert ("Drift wird zurückgedreht"), nicht nur eine andere Art zu
deployen.

## Verwendung (geplant)

```bash
kubectl apply -f gitops/apps/app-of-apps.yaml
argocd app list
argocd app get ticketflow
```

`TODO`: Application-Manifeste schreiben, sobald Chart-Versionen/Repo-URL final feststehen.
