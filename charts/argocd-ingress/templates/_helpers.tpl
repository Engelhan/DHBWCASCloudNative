{{/* argocd.<dnsZone> unless overridden (must stay inside the RFC2136 zone). */}}
{{- define "argocd-ingress.hostname" -}}
{{- .Values.hostname | default (printf "argocd.%s" (required "ticketflow.dnsZone is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.dnsZone)) -}}
{{- end -}}

{{/* CNAME target: Traefik's hostname published by charts/traefik-config. */}}
{{- define "argocd-ingress.externalDnsTarget" -}}
{{- .Values.externalDnsTarget | default (printf "traefik.%s" (required "ticketflow.dnsZone is missing - pass gitops/env/cluster.yaml (-f gitops/env/cluster.yaml)" .Values.ticketflow.dnsZone)) -}}
{{- end -}}
