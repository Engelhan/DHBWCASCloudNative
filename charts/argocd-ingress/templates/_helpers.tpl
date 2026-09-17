{{/*
Hostname der ArgoCD-UI. Leitet sich per Default aus der zentralen DNS-Zone ab
(gitops/env/cluster.yaml -> ticketflow.dnsZone), damit ein Fork ihn nicht separat pflegen muss.
values.yaml -> hostname ueberschreibt ihn, falls doch ein abweichender Name gewuenscht ist - er
muss dann weiterhin INNERHALB der aktivierten RFC2136-Zone liegen, sonst kann weder cert-manager
(dns01) noch external-dns dafuer Zertifikat/Record ausstellen.
*/}}
{{- define "argocd-ingress.hostname" -}}
{{- .Values.hostname | default (printf "argocd.%s" (required "ticketflow.dnsZone fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.dnsZone)) -}}
{{- end -}}

{{/*
CNAME-Ziel fuer den DNS-Record dieser IngressRouteTCP - der von charts/traefik-config auf
Traefiks eigenem LoadBalancer-Service veroeffentlichte Hostname. Beide leiten sich aus derselben
zentralen Zone ab, koennen also nicht mehr auseinanderlaufen (frueher zwei getrennt gepflegte
Literale).
*/}}
{{- define "argocd-ingress.externalDnsTarget" -}}
{{- .Values.externalDnsTarget | default (printf "traefik.%s" (required "ticketflow.dnsZone fehlt - gitops/env/cluster.yaml mitgeben (-f gitops/env/cluster.yaml)" .Values.ticketflow.dnsZone)) -}}
{{- end -}}
