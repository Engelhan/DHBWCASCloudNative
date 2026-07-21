# charts/ – Helm-Packaging (W1)

Enthält den eigenen Umbrella-Chart [`ticketflow/`](ticketflow/), der die gesamte Anwendungsebene
bündelt: die drei eigenen Services, die CNPG-`Cluster`-Ressource und die Strimzi-`Kafka`-Ressource
sowie zugehörige ConfigMaps/Secrets/ServiceMonitors/HPA-KEDA-Objekte.

Die Cluster-weiten Operatoren selbst (CNPG-Operator, Strimzi-Operator, KEDA,
kube-prometheus-stack) sind **nicht** Teil dieses Charts, sondern eigene ArgoCD-`Application`s in
[`../gitops/apps/`](../gitops/apps/) – ein Operator ist Cluster-Infrastruktur, keine
Anwendungs-Konfiguration, und soll unabhängig vom App-Chart aktualisierbar sein.
