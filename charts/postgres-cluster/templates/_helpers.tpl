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
Credentials-Secret (Keys username/password). Wird von der Cluster-CR als bootstrap-Secret genutzt
UND von catalog-api/reservation-worker per secretKeyRef gelesen - beide liegen seit der
Namespace-Zusammenlegung im selben Namespace wie dieser Chart.
*/}}
{{- define "postgres-cluster.userSecretName" -}}
{{- printf "%s-postgres-user" (include "postgres-cluster.databaseName" .) -}}
{{- end -}}
