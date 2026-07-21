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

Die eine Ausnahme ist [`cert-manager-config/`](cert-manager-config/): kein Operator-Wrapper,
sondern unsere **eigene** Custom Resource (ein `ClusterIssuer`, den es upstream nicht als Chart
gibt) – analog zur CNPG-`Cluster`-CR in `ticketflow/`, nur eben nicht Teil des ticketflow-Charts,
weil sie zum Addon-Layer statt zur TicketFlow-Anwendung gehört. Siehe
[`../gitops/README.md`](../gitops/README.md) für den Aktivierungs-Ablauf (die Werte sind aktuell
Platzhalter).
