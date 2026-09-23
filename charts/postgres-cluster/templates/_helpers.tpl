{{/*
Zentrale Ableitungen aus gitops/env/cluster.yaml.

Der logische Datenbankname steht dort (ticketflow.postgres.databaseName), weil er nicht nur diesen
Chart betrifft: catalog-api und reservation-worker leiten daraus den rw-Service und den Namen des
Credentials-Secrets ab. Stuende er nur hier, muessten drei Charts gleichzeitig stimmen.
*/}}
{{- define "postgres-cluster.databaseName" -}}
{{- required "ticketflow.postgres.databaseName fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.postgres.databaseName -}}
{{- end -}}

{{/* Name der CNPG-Cluster-CR. */}}
{{- define "postgres-cluster.clusterName" -}}
{{- printf "%s-postgres-cluster" (include "postgres-cluster.databaseName" .) -}}
{{- end -}}

{{/*
Konventioneller Credentials-Secret-Name (Keys username/password), von catalog-api/
reservation-worker per secretKeyRef gelesen - beide liegen seit der Namespace-Zusammenlegung im
selben Namespace wie dieser Chart. Seit B1 (Root-README ## Bonus) NICHT mehr die Cluster-CRs
eigenes bootstrap-Secret (siehe postgres-cluster.appSecretName unten) - dieses hier wird per
ExternalSecret aus OpenBao befuellt (templates/postgres-user-secret.yaml), damit sich am
Consumer-seitigen Namen nichts aendert, obwohl CNPG das eigentliche Credential jetzt selbst
generiert.
*/}}
{{- define "postgres-cluster.userSecretName" -}}
{{- printf "%s-postgres-user" (include "postgres-cluster.databaseName" .) -}}
{{- end -}}

{{/*
Das von CNPG SELBST generierte App-User-Secret (seit bootstrap.initdb kein eigenes secret-Feld
mehr vorgibt, siehe postgres-cluster.yaml) - Keys username/password/host/port/dbname/uri/
jdbc-uri, CNPG-Namenskonvention "<clusterName>-app". templates/postgres-user-pushsecret.yaml
liest dieses Secret und spiegelt es nach OpenBao; von dort synct
templates/postgres-user-secret.yaml es unter dem konventionellen Namen
(postgres-cluster.userSecretName) zurueck, den catalog-api/reservation-worker erwarten.
*/}}
{{- define "postgres-cluster.appSecretName" -}}
{{- printf "%s-app" (include "postgres-cluster.clusterName" .) -}}
{{- end -}}
