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
Das von CNPG SELBST generierte App-User-Secret (bootstrap.initdb gibt bewusst kein eigenes
secret-Feld vor, siehe postgres-cluster.yaml) - Keys username/password/host/port/dbname/uri/
jdbc-uri, CNPG-Namenskonvention "<clusterName>-app". catalog-api/reservation-worker lesen es
direkt per secretKeyRef (selber Namespace); templates/postgres-user-pushsecret.yaml spiegelt es
zusaetzlich nach OpenBao - nur als Audit-Kopie, kein Konsument liest von dort zurueck.
*/}}
{{- define "postgres-cluster.appSecretName" -}}
{{- printf "%s-app" (include "postgres-cluster.clusterName" .) -}}
{{- end -}}
