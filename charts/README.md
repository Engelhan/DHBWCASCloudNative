# charts/ – Helm-Packaging (W1)

Enthält den eigenen Umbrella-Chart [`ticketflow/`](ticketflow/), der die gesamte Anwendungsebene
bündelt: die drei eigenen Services, die CNPG-`Cluster`-Ressource und die Strimzi-`Kafka`-Ressource
sowie zugehörige ConfigMaps/Secrets/ServiceMonitors/HPA-KEDA-Objekte.

Die Cluster-weiten Operatoren selbst (CNPG-Operator, Strimzi-Operator, KEDA,
kube-prometheus-stack, sowie die Addons Longhorn/cert-manager/external-dns) sind **nicht** Teil
dieses Ordners, sondern eigene ArgoCD-`Application`s in [`../gitops/apps/`](../gitops/apps/), die
direkt auf den jeweiligen Upstream-Helm-Repo zeigen – ein Operator/Addon ist Cluster-
Infrastruktur, keine Anwendungs-Konfiguration, und soll unabhängig vom App-Chart aktualisierbar
sein.

Zwei Ausnahmen davon, aus demselben Grund: kein Operator-Wrapper, sondern unsere **eigenen**
Custom Resources, die es upstream nicht als Chart gibt – analog zur CNPG-`Cluster`-CR in
`ticketflow/`, nur eben nicht Teil des ticketflow-Charts, weil sie zum Addon-Layer statt zur
TicketFlow-Anwendung gehören:

- [`cert-manager-config/`](cert-manager-config/): `ClusterIssuer` fürs ACME/DNS-01-Setup. Siehe
  [`../gitops/README.md`](../gitops/README.md) ## DNS/ACME für die noetige
  TSIG-Secret-Voraussetzung (das Secret selbst steht bewusst nicht im Repo).
- [`argocd-ingress/`](argocd-ingress/): `Certificate` + Traefik `IngressRouteTCP`, macht die
  ArgoCD-UI über die echte DHBW-DNS-Zone erreichbar. Siehe
  [`../gitops/README.md`](../gitops/README.md) ## ArgoCD-UI öffentlich erreichbar.
