# Screencast-Drehbuch

Eine durchgehende Aufnahme: **frisches Projekt → `tofu apply` → `ansible-playbook` → Demo am
neu gebauten Cluster.** Der Aufbau wird im Schnitt beschleunigt, die Demo läuft in Echtzeit.

**Roter Faden:** so, wie der Stack entsteht und benutzt wird — Infrastruktur → Plattform
(Platzierung, Härtung, Secrets, Kafka) → Anwendung → Ausfall → Ansturm → Betrieb per Git. Jedes
Kapitel benutzt etwas aus dem vorherigen: Das AKHQ-Passwort kommt aus OpenBao (Kap. 4 → 5), die
Buchung aus Kap. 6 muss den Ausfall in Kap. 7 überleben, und den Kafka-Lag in Kap. 8 zeigt das
AKHQ-Fenster, das seit Kap. 5 eingeloggt ist.

Vorgaben (Aufgabenstellung V2, 6.3 / Teil C): ≤ 7:00 · Kapitelmarken · jedes Mitglied spricht ·
Live-Terminal, keine Folien · Pflichtszene live (Kap. 7) · B2: Last → Replicas wachsen (Kap. 8).

---

## Regieplan (eine Seite)

**V** = Video-Zeit im fertigen Schnitt · **Ton:** *live* = beim Aufnehmen sprechen,
*VO* = Voiceover, später über den beschleunigten Teil gesprochen · Befehle `[2a]` usw. stehen
unten unter [Befehle und Sprechtexte](#befehle-und-sprechtexte).

| # | V | Kapitel · Sprecher | Aufnahme: was tun | Ton | Schnitt |
|---|---|---|---|---|---|
| 0 | — | *Vorbereitung* | [Checkliste](#vor-der-aufnahme-nicht-im-bild) abhaken, OBS starten, **Uhrzeit notieren** | — | vor V 0:00 alles weg |
| 1 | 0:00 | **Problem und Ausgangslage** · Sturm | `[1a]`: Repo und `cluster.yaml` zeigen | live | 1× |
| 2 | 0:25 | **Aufbau aus dem Repo** (P2, W2) · Niklas | `[2a]` tofu → `[2b]` ansible; währenddessen nichts tippen. Sobald die Stufe `platform` wartet, optional ArgoCD-UI zeigen. Wenn das Playbook grün ist: `[2c]` | VO | tofu ~12×, ansible ~40× (Faktor = Rohdauer / Zielzeit). Hinweis „Zeitraffer“ einblenden |
| 3 | 1:35 | **Platzierung über drei Regionen** (P4) · Sturm | `[3a]` | live | 1× |
| 4 | 2:05 | **Härtung: Deny-by-default und OpenBao** (B3/W7) · Sturm | `[4a]` NetworkPolicy-Probe → `[4b]` OpenBao-Pods und ExternalSecrets → `[4c]` Root-Token ausgeben → Browser `openbao.<zone>`: Login per Token, `secret/` durchklicken, `akhq/basic-auth` öffnen, Passwort kopieren | live | Wartezeit auf busybox (Timeout) ggf. 2× |
| 5 | 2:50 | **Kafka mit Strimzi** (B4) · Carlos | Browser: `akhq.<zone>`, Login `admin` + Passwort aus der OpenBao-UI, Topics/Consumer Groups zeigen, Tab offen lassen. Terminal: `[5a]` Zugangsdaten holen → `[5b]` ACL-Probe | live | Image-Pull ggf. 2× |
| 6 | 3:30 | **Die Anwendung** (P1, P3) · Philipp | `[6a]`; dann Browser `ticketflow.<zone>`: einen Platz buchen, warten bis „bestätigt“; `[6b]` Reservierung in Postgres | live | 1× |
| 7 | 4:00 | **Pflichtszene: Pod-Verlust, Daten bleiben** (P5) · Philipp + Niklas | 2. Terminal: `[7a]` (watch). 1. Terminal: `[7b]` alles löschen (Niklas). Warten bis alles `Running` ist. `[7c]` dieselbe Reservierung abfragen; im Browser einen zweiten Platz buchen (Philipp) | live | Warten 3–4×, Löschen und Ergebnis 1× |
| 8 | 5:15 | **Flash Sale: Last, KEDA, Rolling Update** (B2, P5) · Carlos | 2. Terminal: `[8a]` (watch). 1. Terminal: `[8b]` k6 starten. Nach ~20 s AKHQ-Tab (Lag steigt) → zurück → `[8c]` → `[8d]` Rollout während der Last → k6-Zusammenfassung abwarten | live | Lastlauf 2× |
| 9 | 6:20 | **GitOps: Drift wird zurückgedreht** (W2) · Niklas | 2. Terminal: `[9a]` (watch). 1. Terminal: `[9b]` Drift erzeugen, warten bis wieder 2/2. Schlusssatz | live | Warten bis Self-Heal ggf. 2× |
| — | 6:50 | Ende | OBS stoppen | — | 10 s Puffer bis 7:00 |

Sprechanteile: Sturm 1, 3, 4 · Niklas 2, 7b, 9 · Carlos 5, 8 · Philipp 6, 7a/c (Vorschlag).

---

## Vor der Aufnahme (nicht im Bild)

- [ ] Alter Stand ist weg: `cd infra/terraform && tofu destroy` (frisches Projekt = P2-Nachweis)
- [ ] `terraform.tfvars`, `infra/ansible/secrets.yml`, `infra/ansible/argocd_deploy_key` vorhanden
- [ ] Die Images zum `imageTag` in `gitops/env/cluster.yaml` existieren auf ghcr.io und sind
      public, und alles ist gepusht (ArgoCD liest nur den Remote-Stand)
- [ ] Im DHBW-Netz oder VPN (ACME über `certificates.dhbw.cloud`)
- [ ] Der komplette Ablauf wurde **einmal geprobt**, vor allem `[5b]`, `[7b]` und `[9b]`
- [ ] Terminal: Schrift ≥ 18 pt, zweites Terminal (Split) für die `-w`-Ausgaben, venv aktiv:

```bash
cd <repo-root> && source .venv/bin/activate
export KUBECONFIG=$PWD/infra/ansible/kubeconfig   # die Datei entsteht erst durch Ansible
ZONE=$(sed -n 's/^ *dnsZone: *//p' gitops/env/cluster.yaml)
clear
```

- [ ] OBS: 1920×1080, 30 fps, Mikrofone gepegelt. Wer nicht am Rechner sitzt, spricht über Call
      mit (eigene Tonspur) oder spricht sein Voiceover später ein.
- [ ] Notizzettel für die Schnittliste: Uhrzeit bei Start/Ende jedes Kapitels mitschreiben

---

## Befehle und Sprechtexte

Sprechtexte sind Richtwerte (~130 Wörter/Minute) und sollen sinngemäß gesprochen, nicht
abgelesen werden.

### 1 · Problem und Ausgangslage · Sturm · 0:00–0:25

```bash
# [1a]
ls && sed -n '1,30p' gitops/env/cluster.yaml
```

> „Das ist TicketFlow, eine Ticketplattform für Live-Events. Beim Vorverkauf eines großen Konzerts
> wollen tausende Menschen in denselben Sekunden einen von wenigen Plätzen. Bucht man synchron in
> die Datenbank, wird sie zum Flaschenhals, und Plätze werden im Zweifel doppelt verkauft. Bei uns
> nimmt eine API die Anfrage nur an und legt sie in Kafka ab, Worker buchen im Hintergrund. Wir
> starten mit einem leeren DHBWCloud-Projekt — alles kommt aus diesem Repository.“

### 2 · Aufbau aus dem Repo · Niklas · 0:25–1:35 · Zeitraffer + Voiceover

```bash
# [2a]
cd infra/terraform && tofu init && tofu apply -auto-approve
# [2b]
cd ../ansible && ansible-playbook playbooks/site.yml
# [2c] - wenn das Playbook durch ist
cd ../.. && kubectl get nodes -o wide && kubectl -n argocd get applications -L ticketflow.io/stage
```

> **VO:** „OpenTofu legt vier VMs an — eine Control Plane, drei Worker — und erzeugt daraus das
> Ansible-Inventar. Ansible installiert k3s im Dual-Stack, setzt die Region-Labels und
> bootstrappt ArgoCD, mehr nicht. Ab da gilt GitOps: Ansible wendet zwei ApplicationSets an,
> zuerst die Plattform mit Speicher, Zertifikaten, Operatoren und dem Secret-Vault. Erst wenn alle
> Plattform-Apps grün sind, folgt die Anwendung — sonst würde ein frischer Cluster minutenlang in
> Fehlschleifen laufen. Nach rund vierzig Minuten Echtzeit: 21 Applications, alle Synced und
> Healthy, ohne einen Klick im Portal.“

### 3 · Platzierung über drei Regionen · Sturm · 1:35–2:05

```bash
# [3a]
kubectl get nodes -L topology.ticketflow/region
kubectl -n kafka get pods -l strimzi.io/cluster=ticketflow-kafka -o wide
kubectl -n ticketflow get pods -l cnpg.io/cluster=postgres-postgres-cluster -o wide -L cnpg.io/instanceRole
```

> „Die DHBWCloud hat nur eine Availability Zone, also simulieren wir drei Regionen per Node-Label.
> Die harte Regel kommt aus der Domäne: Fällt mitten im Vorverkauf ein Node aus, dürfen weder die
> Reservierungen noch ihr Event-Log verschwinden. Kafka-Broker und -Controller sowie die drei
> Postgres-Instanzen liegen deshalb je einmal pro Region, und jeder Service läuft mindestens
> zweimal in zwei Regionen.“

Beim Sprechen auf die `NODE`-Spalte zeigen.

### 4 · Härtung: Deny-by-default und OpenBao · Sturm · 2:05–2:50

```bash
# [4a] Ein fremder Pod im App-Namespace erreicht die Datenbank nicht
kubectl -n ticketflow run np-probe --rm -it --restart=Never --image=busybox:1.37 -- \
  nc -zv -w 3 postgres-postgres-cluster-rw 5432

# [4b] Der Vault: drei Instanzen über drei Regionen, Secrets werden nach Kubernetes synchronisiert
kubectl -n openbao get pods -o wide
kubectl get externalsecret -A

# [4c] Root-Token für den UI-Login (einziges menschliches Login, siehe README B3)
kubectl -n openbao get secret openbao-unseal-keys -o jsonpath='{.data.root_token}' | base64 -d; echo
```

Browser: `https://openbao.<zone>` → Methode **Token** → Root-Token einfügen → *Secrets Engines*
`secret/`. Kurz die Ordner zeigen: `akhq/`, `kafka/` (von Strimzi erzeugt und per `PushSecret`
gespiegelt), `postgres/` (von CNPG erzeugt), `notification-worker/`. Dann `akhq/basic-auth`
öffnen, das Passwort per Auge-Symbol aufdecken und kopieren.

> „Jeder Namespace startet mit Deny-by-default, jede Verbindung ist einzeln freigegeben. Ein Pod
> ohne Regel kommt nicht an die Datenbank, der Versuch läuft in einen Timeout. Die Secrets
> liegen in OpenBao, dreifach über die Regionen verteilt und automatisch entsiegelt. Der External
> Secrets Operator synchronisiert sie als normale Kubernetes-Secrets. In der OpenBao-UI sieht man
> jedes Credential des Clusters an einer Stelle: die Kafka-Passwörter von Strimzi, das Postgres-
> Passwort von CloudNativePG, und Passwörter, die der Bootstrap-Job selbst erzeugt hat. Keins davon
> hat ein Mensch vergeben oder ins Git geschrieben. Hier liegt das Login für AKHQ, das kopiere ich
> mir.“

### 5 · Kafka mit Strimzi · Carlos · 2:50–3:30

Browser: `https://akhq.<zone>` → Login `admin` und das Passwort aus der OpenBao-UI → *Topics*
(`reservation-requested` mit 20 Partitionen, RF 3) → *Consumer Groups*. Den Tab offen lassen.

```bash
# [5a] Image des Brokers und die echten Zugangsdaten des notification-worker
KIMG=$(kubectl -n kafka get pod ticketflow-kafka-broker-0 -o jsonpath='{.spec.containers[0].image}')
JAAS=$(kubectl -n ticketflow get secret notification-worker -o jsonpath='{.data.sasl\.jaas\.config}' | base64 -d)

# [5b]
kubectl -n ticketflow get kafkauser
kubectl -n ticketflow run acl-probe -i --rm --restart=Never --image="$KIMG" \
  --labels=app.kubernetes.io/name=notification-worker --env="JAAS=$JAAS" -- bash -c \
  'echo fake | /opt/kafka/bin/kafka-console-producer.sh \
     --bootstrap-server ticketflow-kafka-kafka-bootstrap.kafka.svc:9092 --topic reservation-requested \
     --producer-property security.protocol=SASL_PLAINTEXT \
     --producer-property sasl.mechanism=SCRAM-SHA-512 \
     --producer-property "sasl.jaas.config=$JAAS"'
```

Erwartete Ausgabe: `TopicAuthorizationException: Not authorized to access topics: [reservation-requested]`.

> „Mit dem Passwort aus OpenBao sind wir in AKHQ, schreibgeschützt. Kafka betreiben wir deklarativ
> über Strimzi: Jeder Service hat einen eigenen `KafkaUser` mit eigenem Passwort und engen ACLs,
> alles aus Git. Jetzt versucht der Notification-Worker mit seinen echten Zugangsdaten, eine
> Reservierung einzuschleusen. Kafka lehnt das hart ab. Ein Bug fällt so sofort auf, statt still
> falsche Buchungen zu erzeugen.“

Das Label am Probe-Pod ist nötig, weil der Pod sonst schon an der NetworkPolicy aus Kapitel 4
scheitert. Dann gäbe es einen Timeout statt des ACL-Fehlers.

### 6 · Die Anwendung · Philipp · 3:30–4:00

```bash
# [6a]
kubectl -n ticketflow get deploy
kubectl -n ticketflow get deploy ticketflow-app-catalog-api -o jsonpath=\
'{.spec.template.spec.containers[0].image}{"\n"}{.spec.template.spec.containers[0].resources}{"\n"}{.spec.template.spec.containers[0].envFrom}{"\n"}'
```

Browser: `https://ticketflow.<zone>` → Platz wählen → buchen → Status „bestätigt“.

```bash
# [6b]
kubectl -n ticketflow exec postgres-postgres-cluster-1 -c postgres -- psql -U postgres -d postgres -c \
  "SELECT id, seat_id, status, created_at FROM reservations ORDER BY created_at DESC LIMIT 3;"
```

> „Darauf läuft unsere Anwendung: drei eigene Go-Services. Die Images baut GitHub Actions aus
> `src/` und taggt sie mit dem Commit-Hash, ein `latest` gibt es nicht. Requests und Limits sind
> hergeleitet, die Konfiguration kommt aus ConfigMap und Secret. Ich buche einen Platz — die API
> legt die Anfrage in Kafka ab, der Reservation-Worker bucht sie, und die Reservierung steht
> bestätigt in Postgres.“

### 7 · Pflichtszene: Pod-Verlust, Daten bleiben · Philipp + Niklas · 4:00–5:15

```bash
# [7a] - zweites Terminal
kubectl -n ticketflow get pods -w

# [7b] - Niklas
PRIMARY=$(kubectl -n ticketflow get pods -l cnpg.io/cluster=postgres-postgres-cluster,cnpg.io/instanceRole=primary -o name)
kubectl -n ticketflow delete $PRIMARY --wait=false
kubectl -n ticketflow delete pod -l app.kubernetes.io/name=reservation-worker --wait=false
kubectl -n kafka delete pod ticketflow-kafka-broker-0 --wait=false

# [7c] - Philipp, sobald alles wieder Running ist
kubectl -n ticketflow get pods -l cnpg.io/cluster=postgres-postgres-cluster -L cnpg.io/instanceRole
kubectl -n ticketflow exec postgres-postgres-cluster-2 -c postgres -- psql -U postgres -d postgres -c \
  "SELECT id, seat_id, status, created_at FROM reservations ORDER BY created_at DESC LIMIT 3;"
```

> **Niklas:** „Jetzt der Ernstfall: Ich lösche gleichzeitig den Postgres-Primary, beide
> Reservation-Worker und einen Kafka-Broker. CloudNativePG befördert eine Replica zum neuen
> Primary, die Worker startet das Deployment neu, und Kafka arbeitet mit zwei von drei Brokern
> weiter — genau das erlaubt `min.insync.replicas` gleich zwei.“
>
> **Philipp:** „Ein anderer Pod ist jetzt Primary, und meine Buchung von eben ist unverändert da.
> Die Daten liegen auf replizierten Volumes, nicht im Pod. Und eine neue Buchung läuft auch
> schon wieder durch.“ *(im Browser zweiten Platz buchen)*

### 8 · Flash Sale: Last, KEDA, Rolling Update · Carlos · 5:15–6:20

```bash
# [8a] - zweites Terminal
kubectl -n ticketflow get pods -l app.kubernetes.io/name=reservation-worker -w
# [8b]
VUS=1000 DURATION=2m src/load-generator/run.sh
# [8c] - nach ~20 s, nach dem Blick in AKHQ
kubectl -n ticketflow get scaledobject,hpa
# [8d] - während die Last noch läuft
kubectl -n ticketflow rollout restart deploy ticketflow-app-catalog-api
kubectl -n ticketflow rollout status deploy ticketflow-app-catalog-api
```

> „Jetzt der eigentliche Lastpfad: k6 schickt tausend gleichzeitige Käufer, ein Fünftel davon
> zielt absichtlich auf denselben Platz. In AKHQ wächst der Rückstand der Consumer Group, und genau
> darauf reagiert KEDA, nicht auf die CPU: Pro zehn wartende Nachrichten kommt ein Worker dazu, bis
> maximal zehn. Mitten im Ansturm rolle ich die API neu aus, die Pods werden einzeln ersetzt. Das
> Ergebnis: keine Fehler, nur Bestätigungen und erwartete Konflikte, kein Platz doppelt verkauft.
> Eine Minute nach dem Ansturm skaliert KEDA wieder herunter.“

### 9 · GitOps: Drift wird zurückgedreht · Niklas · 6:20–6:50

```bash
# [9a] - zweites Terminal
kubectl -n ticketflow get deploy ticketflow-app-catalog-api -w
# [9b]
kubectl -n ticketflow scale deploy ticketflow-app-catalog-api --replicas=5
kubectl -n argocd get app ticketflow-app
```

> „Zum Schluss der Betrieb. Angenommen, jemand skaliert während eines Incidents per Hand auf
> fünf Replicas, am Git vorbei. ArgoCD erkennt die Abweichung und dreht sie per Self-Heal zurück
> auf zwei. Was im Cluster läuft, bestimmt nur das Repository — derselbe Weg, auf dem wir den
> ganzen Stack am Anfang aufgebaut haben. Das war TicketFlow, danke!“

---

## Nach der Aufnahme: Schnitt

1. **Schnittliste** aus den notierten Uhrzeiten erstellen (Rohzeit In/Out je Kapitel).
2. **Kapitel 2 beschleunigen.** Am einfachsten im Videoschnitt (Kdenlive, Shotcut, DaVinci) per
   Geschwindigkeitsänderung des Clips. Mit ffmpeg geht es so (Ton fällt weg, das Voiceover kommt
   danach darüber):
   ```bash
   # Faktor = Rohdauer / Zieldauer, z. B. 40 min Ansible auf 50 s -> 48
   ffmpeg -ss 00:01:10 -to 00:41:30 -i raw.mkv -an -vf "setpts=PTS/48" -r 30 kap2-ansible.mp4
   ```
3. **Voiceover** für Kapitel 2 einsprechen und über den Zeitraffer legen. Dazu den Hinweis
   „Zeitraffer ×48“ als Textzeile einblenden: Das ist ehrlich, und es bleibt eine echte
   Live-Aufnahme.
4. **Kapitelmarken** mit den tatsächlichen Startzeiten in `screencast/chapters.txt` eintragen:

```ini
;FFMETADATA1
[CHAPTER]
TIMEBASE=1/1000
START=0
END=25000
title=1 Problem und Ausgangslage

[CHAPTER]
TIMEBASE=1/1000
START=25000
END=95000
title=2 Aufbau aus dem Repo (Zeitraffer)

[CHAPTER]
TIMEBASE=1/1000
START=95000
END=125000
title=3 Platzierung über drei Regionen

[CHAPTER]
TIMEBASE=1/1000
START=125000
END=170000
title=4 Härtung: Deny-by-default und OpenBao

[CHAPTER]
TIMEBASE=1/1000
START=170000
END=210000
title=5 Kafka mit Strimzi: Identität und ACLs je Service

[CHAPTER]
TIMEBASE=1/1000
START=210000
END=240000
title=6 Die Anwendung

[CHAPTER]
TIMEBASE=1/1000
START=240000
END=315000
title=7 Pflichtszene: Pod-Verlust, Daten bleiben

[CHAPTER]
TIMEBASE=1/1000
START=315000
END=380000
title=8 Flash Sale: Last, KEDA, Rolling Update

[CHAPTER]
TIMEBASE=1/1000
START=380000
END=410000
title=9 GitOps: Drift wird zurückgedreht
```

5. **Exportieren und Kapitel einbetten:**

```bash
ffmpeg -i schnitt.mkv -i screencast/chapters.txt -map_metadata 1 -map_chapters 1 \
  -vcodec libx264 -crf 28 -preset slower -acodec aac -b:a 128k screencast/demo.mp4
ffprobe -v error -show_chapters screencast/demo.mp4
ffprobe -v error -show_entries format=duration -of csv=p=0 screencast/demo.mp4   # < 420
```

6. Die Kapitelliste mit Zeitstempeln zusätzlich in `screencast/README.md` eintragen.

### Abnahme

- [ ] Länge ≤ 7:00, Kapitelmarken im MP4 und in `screencast/README.md`
- [ ] Alle vier Mitglieder sind zu hören
- [ ] Pflichtszene vollständig: löschen → erholt → dieselbe Reservierung noch da
- [ ] `[4a]` endet mit Timeout, `[5b]` mit `TopicAuthorizationException` (nicht Timeout)
- [ ] Repo inklusive Video < 100 MB
