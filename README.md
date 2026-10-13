# TicketFlow

TicketFlow ist eine cloud-native Ticketverkaufsplattform für Live-Events.


## Problem

Beim Vorverkauf populärer Events (z. B. Konzert, Fußballspiel) starten tausende Nutzer innerhalb
weniger Sekunden denselben Kaufversuch für eine kleine, feste Anzahl an Plätzen. Ein klassischer
synchroner CRUD-Ansatz (Request → sofortiger DB-Write) macht die Datenbank zum Flaschenhals:
Nutzer sehen Timeouts statt einer fairen Reihenfolge und Plätze werden im Zweifel doppelt
vergeben. TicketFlow trennt die *Annahme* einer Reservierungsanfrage von deren
*Verarbeitung* über einen Message Bus und skaliert die Verarbeitung anhand der Warteschlangenlänge
nach. Genau dieser Lastpfad ist der fachliche Grund, warum Queue, Datenbank und Autoscaling hier tatsächlich gebraucht werden.

## Architektur

![TicketFlow-Architektur](docs/architecture/architecture.drawio.svg)

Fachlicher Lastpfad (durchgezogene Pfeile): `catalog-api` nimmt die Reservierung an und
veröffentlicht sie nur (①), `reservation-worker` sperrt den Platz in Postgres (②) und meldet das
Ergebnis zurück auf Kafka (③). Dieses Ergebnis liest der `notification-worker` als zweite,
unabhängige Consumer-Group. Gestrichelt: Steuerung und Plattform (GitOps, Secrets, Autoscaling, DNS/TLS). Die Datei ist
ein draw.io-SVG mit eingebettetem Diagramm. Man kann sie direkt in [draw.io](https://app.diagrams.net) oder
der VS-Code-Erweiterung „Draw.io Integration" öffnen und bearbeiten, GitHub rendert sie als Bild.

**Datenfluss von der VM bis zum Pod:**

1. Terraform/OpenTofu erzeugt gegen den OpenStack-Provider der DHBWCloud vier VMs (1
   Control-Plane, 3 Worker) direkt am vorhandenen, extern routbaren Projektnetz `DHBWV6`. Es gibt kein
   eigenes Netz, keinen Router und keine selbst verwaltete Security Group, siehe
   [`infra/terraform/`](infra/terraform/). Jeder Worker erhält eine virtuell simulierte
   Regions-Metadata (`region-a`/`region-b`/`region-c`), da die DHBWCloud real nur eine
   Region/AZ bereitstellt.

2. Ansible konfiguriert die VMs, installiert k3s (Server auf der Control-Plane, Agents auf den
   Workern), übernimmt die simulierte Region als Kubernetes-Node-Label
   (`topology.ticketflow/region`) und bootstrapt ArgoCD, siehe [`infra/ansible/`](infra/ansible/).

3. ArgoCD übernimmt ab hier per GitOps den Rest. Zwei `ApplicationSets`
   ([`gitops/applicationsets/`](gitops/applicationsets/)) erzeugen gemeinsam alle 21
   Applications, und zwar in zwei Stufen. Zuerst kommt die Plattform: Cluster-Addons (Longhorn,
   cert-manager, external-dns), Operatoren (CNPG, Strimzi, KEDA, External Secrets) und der
   zentrale Secret-Vault OpenBao. Erst wenn deren Pods alle Health-Checks bestehen, erzeugt ArgoCD
   die Anwendungsstufe. Sie besteht aus Datenhaltung, Kafka, den projektweiten NetworkPolicies
   (siehe `## Bonus`) und den TicketFlow-Charts selbst. Alle deployment-spezifischen Werte stammen
   aus der zentralen [`gitops/env/cluster.yaml`](gitops/env/cluster.yaml), siehe
   [`gitops/`](gitops/).

4. `catalog-api` nimmt Reservierungsanfragen entgegen und veröffentlicht sie als Event auf Kafka,
   statt synchron in die Datenbank zu schreiben.

5. `reservation-worker` konsumiert diese Events, sperrt den Sitzplatz transaktional in CNPG-Postgres
   und veröffentlicht das Ergebnis (bestätigt/abgelehnt) wieder auf Kafka.

6. `notification-worker` konsumiert das Ergebnis und simuliert eine Bestätigungs-Benachrichtigung.

7. Der `load-generator` erzeugt künstlich die namensgebende Lastspitze. KEDA reagiert auf den
   dadurch wachsenden Kafka-Consumer-Lag und skaliert den `reservation-worker` hoch. Dies ist sichtbar über
   `kubectl get pods`/`scaledobject` und den Consumer-Lag in AKHQ (siehe `## Betrieb`/`## Bonus`).
   Ein Grafana-Dashboard gibt es nicht.

**Pod-Verteilung im laufenden Cluster: ein Beispiel** (realer Abzug vom 09.10.2026, dev-Profil,
aus `kubectl get pods -A -o wide`). Welcher Pod auf welchem Node landet, ändert sich bei jedem
Neustart oder Rescheduling. Fest sind nur die Platzierungsregeln, die das Bild sichtbar macht:

![Pod-Verteilung auf die Nodes](docs/architecture/pod-distribution.drawio.svg)

Kafka-Broker/-Controller und OpenBao liegen erzwungen je einmal pro simulierter Region,
die drei App-Postgres-Pods (Primary + zwei Replicas) ebenso, und jeder weitere Dienst läuft mit
mindestens zwei Pods in zwei Regionen (siehe `## Entscheidungen`, P4). Auf der getainteten
Control-Plane laufen nur k3s' eigene Komponenten.

## Betrieb

Dieser Abschnitt beschreibt Schritt für Schritt, wie der komplette Stack in einem **leeren
DHBWCloud-Projekt** unter einer **eigenen DNS-Zone** neu aufgebaut wird, ohne Zugänge aus dem
ursprünglichen Setup zu übernehmen. Alle Zugänge werden neu beschafft. Das Repo enthält kein
Geheimnis, das wiederverwendet werden müsste.

Der Ablauf folgt einem festen roten Faden:

```
Voraussetzungen ─▶ Zugänge beschaffen ─▶ Konfiguration (Git) ─▶ Images bauen
     ─▶ Geheimnisse (lokal) ─▶ terraform apply ─▶ ansible-playbook ─▶ ArgoCD übernimmt
     ─▶ Prüfen ─▶ Lastpfad demonstrieren
```

Jeder Wert, der sich zwischen zwei Deployments unterscheidet, steht an genau einer von zwei Stellen:

| Art | Datei | In Git? | Inhalt |
|---|---|---|---|
| Konfiguration | [`gitops/env/cluster.yaml`](gitops/env/cluster.yaml) | ja | Fork-URL, DNS-Zone, ACME-Kontakt, TSIG-Key-*Name*, Registry, Image-Tag, Umgebung, Namenspräfix |
| Geheimnisse | `infra/terraform/terraform.tfvars`, `infra/ansible/secrets.yml`, `infra/ansible/argocd_deploy_key` | nein (gitignored) | OpenStack-Credential, TSIG-Secret-*Wert*, ArgoCD-Deploy-Key |

`cluster.yaml` wird von Terraform, Ansible, ArgoCD und allen Helm-Charts gelesen. Kein Wert daraus
steht an anderer Stelle noch einmal als Literal. Die einzige Ausnahme ist die Repo-URL in den
beiden ApplicationSets (Schritt 8). Alle Passwörter *innerhalb* des Clusters (Datenbanken,
Kafka-User, AKHQ, OpenBao) werden beim ersten Rollout im Cluster selbst erzeugt (CNPG, Strimzi,
OpenBao-Bootstrap-Job) und müssen nicht mitgebracht werden.

### Voraussetzungen

**Konten und Berechtigungen**

| Voraussetzung | Wofür |
|---|---|
| GitHub-Account | privater Fork, GitHub Actions, Container-Registry `ghcr.io` |
| DHBWCloud-Projekt (OpenStack, `newstack.dhbw.cloud`) | VMs, Zugriff auf das Projektnetz `DHBWV6` |
| Quota im Projekt für 4 VMs | 1 Control-Plane + 3 Worker, Worker mit ca. 10 GB RAM, weil allein Kafka 4,5 GiB anfordert |
| Zugang zu `self-service.dhbw.cloud` | eigene DNS-Zone mit RFC2136-Freigabe (TSIG) |

**Netz**

- Lokaler Client im **DHBW-Netz oder per VPN** verbunden. Self-Service-Portal und ACME-Server
  (`certificates.dhbw.cloud`) sind nur von dort erreichbar.
- **IPv6-Konnektivität** zum Netz `DHBWV6`. SSH, Ansible und die Kubeconfig sprechen die VMs über
  ihre IPv6-Adresse an.
- Die **Default-Security-Group** des Projekts muss eingehend SSH (22/tcp), die k3s-API
  (6443/tcp) und HTTP/HTTPS (80, 443/tcp) erlauben sowie Verkehr zwischen den VMs untereinander.
  Terraform verwaltet bewusst keine eigene Security Group (siehe
  [`infra/terraform/README.md`](infra/terraform/README.md)).

**Werkzeuge auf dem lokalen Client**

| Werkzeug | Version | Wofür |
|---|---|---|
| Linux oder WSL | beliebig | Ansible läuft nicht als Controller auf nativem Windows (`ansible-core` scheitert mit `WinError 87`) |
| `git`, `ssh-keygen` | aktuell | Fork klonen, Schlüssel erzeugen |
| Python 3 mit `venv` | wie von `ansible-core` gefordert | Ansible (`ansible==14.2.0`, gepinnt in [`requirements.txt`](infra/ansible/requirements.txt)) |
| Terraform oder OpenTofu | ≥ 1.5 | VMs anlegen. Der Provider `openstack ~> 3.4` wird bei `init` geladen |
| `kubectl` | passend zu k3s `v1.37` | Prüfen des Clusters. Das Playbook legt damit lokal die GitOps-Secrets an |
| Docker | aktuell | Lastgenerator (k6 läuft im Container) |

Alle übrigen Komponenten (k3s, ArgoCD, Helm-Charts) installiert der Ablauf selbst in gepinnten
Versionen. Helm oder das ArgoCD-CLI werden lokal nicht benötigt.

### Phase 1: Repo und Zugänge beschaffen

Diese Schritte lassen sich nicht aus dem Repo heraus automatisieren. Jeder liefert ein Ergebnis,
das in Phase 2 oder 4 eingetragen wird.

1. **Repo privat forken und klonen.** Der Fork muss privat sein (siehe
   [`gitops/README.md`](gitops/README.md) ## Privates Repo). Alle folgenden Befehle laufen im
   Wurzelverzeichnis des Klons.
   ```bash
   git clone git@github.com:<owner>/DHBWCASCloudNative.git && cd DHBWCASCloudNative
   ```
   → *Ergebnis:* SSH-URL des Forks

2. **Python-Umgebung für Ansible anlegen:**
   ```bash
   python3 -m venv .venv && source .venv/bin/activate
   pip install -r infra/ansible/requirements.txt
   ```

3. **OpenStack-Application-Credential anlegen** im DHBWCloud-Dashboard unter *Identity →
   Application Credentials*. Das Secret wird **nur einmal** angezeigt. Im selben Dashboard außerdem
   die Region, den Namen eines Ubuntu-LTS-Images und zwei passende Flavors (Control-Plane, Worker)
   heraussuchen.
   → *Ergebnis:* Credential-ID, Credential-Secret, Region, Image, Flavors

4. **SSH-Keypair für die VMs erzeugen.** Terraform importiert den Public Key als
   OpenStack-Keypair, Ansible meldet sich mit dem Private Key an:
   ```bash
   ssh-keygen -t ed25519 -f ~/.ssh/ticketflow_dhbwcloud -N ""
   ```

5. **DNS-Zone registrieren** auf `self-service.dhbw.cloud`, mit RFC2136-Freigabe (dynamische
   Updates per TSIG). cert-manager löst damit die ACME-DNS-01-Challenge, external-dns legt die
   Hostnamen an.
   → *Ergebnis:* Zonenname, RFC2136-Server, TSIG-Key-**Name**, TSIG-Secret-**Wert**

6. **ArgoCD-Deploy-Key erzeugen** und den Public Key im Fork unter *Settings → Deploy keys* als
   **Read-only** eintragen. Ohne ihn kann ArgoCD das private Repo nicht klonen:
   ```bash
   ssh-keygen -t ed25519 -C "argocd-deploy-key@ticketflow" -f infra/ansible/argocd_deploy_key -N ""
   cat infra/ansible/argocd_deploy_key.pub
   ```

### Phase 2: Fork konfigurieren (git-getrackt)

7. **`gitops/env/cluster.yaml` anpassen.** Die Kommentare in der Datei erklären jedes Feld:

   | Feld | Neuer Wert |
   |---|---|
   | `repoURL` | SSH-URL des Forks (Schritt 1) |
   | `dnsZone` | eigene Zone (Schritt 5), alle Hostnamen leiten sich davon ab |
   | `acmeEmail` | eigene Kontaktadresse für die ACME-Registrierung |
   | `rfc2136.server`, `rfc2136.keyName` | aus Schritt 5 (nur der Key-**Name**, nie das Secret) |
   | `imageRegistry` | `ghcr.io/<github-owner-in-kleinbuchstaben>` |
   | `namePrefix` | eindeutiges Präfix für alle OpenStack-Ressourcen, z. B. `<name>-ticketflow` |
   | `network.nodeCIDRs` | Subnetze von `DHBWV6`, nur prüfen. Sie gelten für jedes Projekt im selben Netz |

   `imageTag` wird in Schritt 10 automatisch gesetzt. `environment` bleibt für den ersten Aufbau
   auf `dev`. `targetRevision` bleibt `main`, solange auf `main` deployt wird.

8. **Repo-URL in den beiden ApplicationSets eintragen** (je eine Zeile im git-Generator von
   [`platform.yaml`](gitops/applicationsets/platform.yaml) und
   [`apps.yaml`](gitops/applicationsets/apps.yaml)). Das ist die einzige Henne-Ei-Stelle. Das
   ApplicationSet muss wissen, aus welchem Repo es `cluster.yaml` liest, bevor es sie lesen kann.

9. **Committen und pushen.** ArgoCD liest ausschließlich den Remote-Stand. Ein lokal geänderter,
   aber nicht gepushter Stand wird nie ausgerollt:
   ```bash
   git commit -am "chore: configure deployment" && git push
   ```

### Phase 3: Images bereitstellen

10. **Images bauen lassen.** In einem frischen Fork sind GitHub Actions deaktiviert und müssen
    unter *Actions* einmal aktiviert werden. Danach den Workflow `build-images` manuell starten
    (`workflow_dispatch`). Er baut die vier Service-Images, pusht sie nach `ghcr.io` und schreibt
    im Job `bump-image-tag` den Short-SHA als `imageTag` in `cluster.yaml`. Den Bot-Commit
    anschließend lokal holen, damit spätere Pushes nicht kollidieren:
    ```bash
    git pull
    ```

11. **Die vier ghcr.io-Packages auf „Public" stellen** (GitHub → *Packages* → jeweils *Package
    settings*). Der eingebaute `GITHUB_TOKEN` kann das nicht selbst. Wegen der öffentlichen Images
    braucht der Cluster keine `imagePullSecret`s.

### Phase 4: Geheimnisse lokal hinterlegen (gitignored)

12. **Vorlagen kopieren und mit den Ergebnissen aus Phase 1 füllen:**
    ```bash
    cp infra/terraform/terraform.tfvars.example infra/terraform/terraform.tfvars
    #  -> os_region, os_application_credential_id/_secret, image_name,
    #     control_plane_flavor, worker_flavor (Schritt 3), SSH-Pfade passen zu Schritt 4
    cp infra/ansible/secrets.yml.example infra/ansible/secrets.yml
    #  -> rfc2136_tsig_secret_value (der WERT aus Schritt 5, nicht der Key-Name)
    ```
    Der Deploy-Key aus Schritt 6 liegt bereits unter `infra/ansible/argocd_deploy_key`. Damit
    sind alle drei gitignorierten Dateien vorhanden.

### Phase 5: Ausrollen

13. **Infrastruktur erzeugen.** Terraform legt die Control-Plane-VM und drei Worker (je eine
    simulierte Region) direkt im Netz `DHBWV6` an und generiert daraus
    `infra/ansible/inventory/hosts.ini`:
    ```bash
    cd infra/terraform
    terraform init
    terraform apply
    ```

14. **Cluster aufbauen und an GitOps übergeben.** Ein einziger Playbook-Lauf konfiguriert die VMs,
    installiert k3s (Dual-Stack, IPv6 primär), setzt Regions-Labels und den Control-Plane-Taint,
    installiert ArgoCD, holt die Kubeconfig nach `infra/ansible/kubeconfig` und legt die
    Repo-Credential sowie die TSIG-Secrets aus Phase 4 an. Zuletzt wendet er die ApplicationSets
    gestuft an: erst `platform` (Addons, Operatoren, OpenBao), und erst wenn alle
    Plattform-Applications *Synced/Healthy* sind, `apps` (Kafka, Postgres, Anwendung). Das
    Playbook endet erst, wenn alles grün ist, oder bricht nach 30 Minuten je Stufe mit einer Liste
    der roten Applications ab:
    ```bash
    cd ../ansible
    ansible-inventory -i inventory/hosts.ini --list   # optional: Inventar prüfen
    ansible-playbook playbooks/site.yml
    ```
    Ab hier gehört der Cluster ArgoCD. Jede weitere Änderung läuft über `git push`, nicht über
    Ansible oder `kubectl`. Das Playbook ist idempotent und kann nach einem Abbruch erneut
    gestartet werden.

### Phase 6: Prüfen und Zugänge abholen

15. **Rollout prüfen** (aus dem Repo-Wurzelverzeichnis):
    ```bash
    cd ../..
    export KUBECONFIG=infra/ansible/kubeconfig
    kubectl get nodes -o wide                                  # 4 Nodes, Worker mit Region-Label
    kubectl -n argocd get applicationset                       # ticketflow-platform, ticketflow-apps
    kubectl -n argocd get applications -L ticketflow.io/stage  # 21 Stück, alle Synced/Healthy
    kubectl get kafka,cluster.postgresql.cnpg.io -A
    ```

16. **Öffentliche Endpunkte aufrufen.** external-dns veröffentlicht die Hostnamen in der eigenen
    Zone, cert-manager stellt die Zertifikate aus:

    | URL | Zugang |
    |---|---|
    | `https://ticketflow.<dnsZone>` | Frontend und API, ohne Login |
    | `https://argocd.<dnsZone>` | `admin`, Passwort steht am Ende der Playbook-Ausgabe (Task „Show the initial admin password") |
    | `https://openbao.<dnsZone>` | Root-Token, siehe unten |
    | `https://akhq.<dnsZone>` | `admin`, nur lesend, Passwort vom OpenBao-Bootstrap zufällig erzeugt, siehe unten |

    ```bash
    # OpenBao-Root-Token und Unseal-Key: nach dem ersten Rollout einmal sicher außerhalb
    # des Clusters ablegen, es gibt bewusst keine weitere Kopie
    kubectl -n openbao get secret openbao-unseal-keys -o yaml
    ROOT_TOKEN=$(kubectl -n openbao get secret openbao-unseal-keys -o jsonpath='{.data.root_token}' | base64 -d)

    # AKHQ-Passwort aus OpenBao lesen
    kubectl -n openbao exec openbao-0 -c openbao -- \
      env BAO_TOKEN="$ROOT_TOKEN" bao kv get -field=password secret/akhq/basic-auth
    ```

### Phase 7: Lastpfad demonstrieren

17. **Flash-Sale auslösen.** Das Skript läuft im k6-Container, ein lokales k6 ist nicht nötig. Die
    Ziel-URL liest es selbst aus `cluster.yaml`:
    ```bash
    cd src/load-generator
    ./run.sh                          # https://ticketflow.<dnsZone>, alle Events, 1000 VUs, 1 Min.
    VUS=2000 DURATION=5m ./run.sh     # mehr Last
    ```

18. **Wirkung beobachten.** Es gibt keinen Observability-Stack im Cluster, die Wirkung zeigen
    `kubectl` und AKHQ:
    ```bash
    kubectl get scaledobject -n ticketflow
    kubectl get pods -n ticketflow -l app.kubernetes.io/name=reservation-worker -w   # KEDA skaliert nach Consumer-Lag
    # Consumer-Lag live in AKHQ: https://akhq.<dnsZone> -> Consumer Groups
    ```

### Typische Fehlerbilder beim Neuaufbau

| Symptom | Ursache | Schritt |
|---|---|---|
| `terraform apply` scheitert an Quota oder unbekanntem Image/Flavor | Projekt-Quota zu klein oder Namen falsch übernommen | 3, 12 |
| Ansible erreicht die VMs nicht (`UNREACHABLE`) | keine IPv6-Route/kein VPN oder Security Group lässt Port 22 nicht zu | Voraussetzungen |
| Alle Applications `Unknown` | Deploy-Key fehlt oder Repo-URL zeigt noch auf das Original-Repo | 6, 7, 8 |
| `external-dns`/`cert-manager` in `CreateContainerConfigError`, Plattform-Stufe wird nie grün | `secrets.yml` fehlt oder enthält noch `CHANGE_ME` | 12 |
| Zertifikate bleiben `False`/`pending` | TSIG-Key-Name oder RFC2136-Server falsch, Zone ohne RFC2136-Freigabe, kein DHBW-Netz | 5, 7 |
| Services im `ImagePullBackOff` | Workflow nie gelaufen, Actions im Fork deaktiviert oder Packages noch privat | 10, 11 |
| Änderung kommt nicht im Cluster an | nicht gepusht, ArgoCD sieht nur den Remote-Stand | 9 |

### Umschalten auf `prod`

Von `dev` auf `prod` (mehr Replicas über alle drei Regionen, engere PodDisruptionBudgets) schaltet
eine einzige Zeile um: `ticketflow.environment` in `gitops/env/cluster.yaml`, danach committen und
pushen. Das ApplicationSet zieht daraufhin für jeden Chart die passende `values-<env>.yaml`.

### Abbau und erneuter Aufbau

```bash
cd infra/terraform && terraform destroy
```

entfernt alle VMs samt Cluster-Inhalt, auch die im Cluster erzeugten Passwörter und das
OpenBao-Root-Token. Konfiguration, Images und die gitignorierten Geheimnisse bleiben erhalten.
Ein erneuter Aufbau beginnt deshalb direkt bei Schritt 13. `hosts.ini` und `kubeconfig` werden
dabei neu erzeugt.

## Entscheidungen

**P1: Anwendung und Domäne.**

*Wahl:* drei eigene Services (`catalog-api`,
`reservation-worker`, `notification-worker`) plus CNPG-Postgres und Strimzi-Kafka als
zustandsbehaftete Dienste.

*Alternative:* ein einzelner Monolith mit synchronem DB-Zugriff.

*Begründung:* Erst die Trennung von Annahme (API) und Verarbeitung (Worker) über eine Queue macht
den Lastpfad des Flash-Sales beherrschbar und erlaubt, genau den Engpass-Service unabhängig zu
skalieren. `catalog-api` ist in Go mit `chi` umgesetzt. Der Dienst schreibt bewusst nie synchron in Postgres,
sondern liest nur und produziert nach Kafka (siehe `src/catalog-api/README.md`). Go bringt dieselbe
Toolchain wie der `reservation-worker` mit, und das Image bleibt im Flash-Sale-Pfad klein und
startet schnell. Eine weitere Alternative wäre Python mit FastAPI (schnelleres Prototyping, aber ein
größeres Image und eine zweite Toolchain neben Go).

*Nachweis im Betrieb:* `kubectl -n ticketflow get deploy,cluster` zeigt die eigenen Services
neben dem CNPG-Cluster, `kubectl -n kafka get kafka,kafkanodepool` das Strimzi-Kafka. Eine Reservierung über das Frontend
(oder `POST /events/{id}/reservations`) antwortet sofort mit `202` und `status: "pending"`.
Wenige Augenblicke später liefert `GET /reservations/{id}` `confirmed` bzw. `rejected`, ohne dass
die API je in Postgres geschrieben hat. In AKHQ hängen am Topic `reservation-requested` die
Consumer-Group `reservation-worker` und am Topic `reservation-result` die Consumer-Group
`notification-worker`. Ein Ereignis durchläuft also drei unabhängige Dienste über Kafka.

**P1: Gemeinsame Datenbank für `catalog-api` und `reservation-worker`.**

*Wahl:* Beide Services nutzen dieselbe CNPG-Datenbank (`ticketflow`) mit demselben, von CNPG generierten
App-User. Die Rollen sind fachlich getrennt: `catalog-api` liest nur (`events`, `seats`,
`reservations` für die Status-Abfrage) und zwar über den Read-Only-Service
`<db>-postgres-cluster-ro`, also von der Replica. `reservation-worker` schreibt als Einziger
(`seats.status`, `reservations`) über `<db>-postgres-cluster-rw` auf den Primary und legt als
Einziger Schema und Stammdaten an (versionierte Migrationen). `notification-worker`
hat keinen Datenbankzugriff.

*Alternative:* „Database per Service“, also eine eigene Datenbank je
Service: Die API hält eine Lese-Kopie der Sitzplatz-Verfügbarkeit, die sie aus den
`reservation-result`-Events aufbaut (CQRS/Read-Model).

*Begründung:* Das knappe Gut im Flash-Sale ist der einzelne Sitzplatz, und dessen Zustand darf es nur einmal geben. Der Worker sperrt ihn
atomar per `UPDATE seats … WHERE status = 'free'` und die API zeigt denselben Datensatz an,
nur um die Streaming-Replikation versetzt (typisch Millisekunden). Mit getrennten Datenbanken hinge
die Sitzplatzanzeige dagegen am Consumer-Lag einer Projektion. Gerade in der Lastspitze, wenn der Consumer-Lag hoch ist (genau dann skaliert KEDA), sähen Käufer längst
vergebene Plätze als frei und würden massenhaft in `409`/`Rejected` laufen. Außerdem bräuchte es
einen zweiten CNPG-Cluster samt Replikation und Backup sowie einen Projektionsdienst. Das ist für
drei Tabellen unverhältnismäßig.

*Vorteile:*
- eine Quelle der Wahrheit für den Sitzplatzstatus, keine Synchronisationslücke zwischen Anzeige
  und Buchung,
- die API kann den Endstatus einer Reservierung direkt aus `reservations` lesen,
- nur ein zustandsbehafteter Datenbankdienst muss betrieben, repliziert (Primary + zwei Replicas in drei
  Regionen, P4/P5) und gesichert werden,
- die Replica, die für P5 ohnehin läuft, trägt die Leselast: Die Abfragen der Seat-Map aus dem
  Frontend konkurrieren in der Lastspitze nicht mit den Buchungen des Workers um den Primary.

*Nachteile:*
- **Schema-Kopplung:** Das Schema gehört allein dem `reservation-worker`
  (`src/reservation-worker/internal/migrate/sql/`), aber die SQL-Abfragen von `catalog-api` hängen
  an denselben Tabellen und Spalten. Eine Schemaänderung muss deshalb rückwärtskompatibel
  ausgerollt werden (erst erweitern, dann beide Services umstellen, dann aufräumen). Unabhängige
  Deployments gelten damit nur, solange das Schema stabil bleibt. Außerdem gibt es auf einem
  frischen Cluster die Tabellen erst, wenn der Worker einmal gelaufen ist.
- **Gemeinsamer Fehlerbereich:** Beide hängen am selben CNPG-Cluster. Schreibend bleibt der eine
  Primary die Skalierungsgrenze des Workers, nicht die Pod-Zahl (siehe B2, `maxReplicaCount`).
  Lesend hängt `catalog-api` an den Replicas. Deshalb läuft der Cluster mit `instances: 3`
  (Primary + zwei Replicas, je eine pro Region): Fällt eine Replica aus oder wird sie beim Failover
  zum Primary befördert, bedient die andere `-ro` weiter. Mit nur einer Replica hätte `-ro` in
  diesem Moment keine Endpoints, und weil `/readyz` die Datenbank prüft, wäre `catalog-api`
  komplett nicht bereit, einschließlich der Annahme von Reservierungen. Der Preis ist ein dritter
  Postgres-Pod mit 2Gi Memory-Request.
- **Replikationsverzug:** Die Replica läuft asynchron hinterher. Ein gerade gebuchter Platz kann
  für Millisekunden noch als frei erscheinen. Der Worker entscheidet aber auf dem Primary, so dass
  höchstens ein zusätzliches `Rejected` entsteht, nie eine Doppelbuchung.
- **Kein Least-Privilege über DB-Rechte:** Beide nutzen denselben DB-User. Dass `catalog-api` nicht
  schreibt, erzwingt nur die Replica, weil sie Schreibzugriffe technisch ablehnt, und kein
  Rechte-Konzept.

*Bezug zur Microservice-Architektur:* Das Muster „Database per Service“ fordert, dass jeder
Service seine Daten exklusiv besitzt und andere nur über seine API oder Events darauf zugreifen.
Diese Regel brechen wir hier bewusst an einer Stelle: Es handelt sich um ein *Shared-Database*-
Muster zwischen zwei Services desselben Bounded Context „Reservierung“. Die Entkopplung, auf die es
im Lastpfad ankommt, bleibt erhalten, denn die API schreibt nie synchron und die Annahme läuft
über Kafka. Damit lassen sich API und Worker unabhängig skalieren und ausfallen. Ist der Worker
weg, nimmt die API weiter an und Kafka puffert. Gekoppelt sind die beiden Services nur über das
gemeinsame Schema, und dieses Schema hat genau einen Besitzer (den Worker als einzigen Schreiber).
Lesepfad (Replica) und Schreibpfad (Primary) sind bereits physisch getrennt, eine abgeschwächte
Form von CQRS innerhalb einer Datenbank. `notification-worker` hält sich dagegen vollständig an das
Microservice-Prinzip und kennt nur Events. Die nächsten Schritte wären ein eigener
Nur-Lese-DB-User für `catalog-api` und erst danach eine eigene Datenbank mit eventbasiertem
Read-Model.

**P2: Infrastruktur als Code.**

*Wahl:* Terraform/OpenTofu gegen den OpenStack-Provider für
VMs/Netz, Ansible für OS-Konfiguration und k3s-Bootstrap. Ein `terraform apply` gefolgt von einem
`ansible-playbook site.yml` bringt den kompletten Stack hoch.

*Alternative:* nur Terraform, ohne Ansible. k3s würde per `cloud-init` (`user_data` der VMs)
installiert, die Worker träten mit einem von Terraform erzeugten Join-Token bei, und ein einziges
`terraform apply` brächte alles hoch.

*Begründung:* `cloud-init` läuft beim ersten Boot genau einmal und gibt keine Rückmeldung.
`terraform apply` meldet „fertig“, sobald die VMs existieren, nicht erst wenn k3s läuft. Ob ein
Worker dem Cluster beigetreten ist, sähe man nur per SSH im Log der jeweiligen VM.

Ansible wartet dagegen aktiv auf das Ergebnis:
- [`k3s_agent`](infra/ansible/roles/k3s_agent/tasks/main.yml) wartet, bis jeder Worker `Ready` ist.
- [`gitops_rollout`](infra/ansible/roles/gitops_rollout/tasks/stage.yml) wartet danach, bis jede
  Application *Synced/Healthy* ist.

Bricht ein Lauf ab, nennt Ansible die rote Stufe. Das Playbook ist idempotent und kann deshalb
gefahrlos erneut starten. `cloud-init` würde dagegen kein zweites Mal laufen.

Für TicketFlow ist das wichtig, weil der Vorverkaufsstart ein fester Termin ist. Vorher muss
belegt sein, dass alle drei Regionen im Cluster sind. Sonst greifen die harten Platzierungsregeln
aus P4 nicht, und Kafka-Broker bleiben `Pending`.

*Trade-off:* Es sind zwei Werkzeuge und zwei Befehle statt einem, dazu kommt ein Python-Venv auf dem
Client. Managed Kubernetes ist auf der DHBWCloud ohnehin ausgeschlossen.

*Abgrenzung:* Terraform legt kein eigenes Netz und keine eigene Security Group an. Die VMs hängen
direkt am vorgegebenen Projektnetz `DHBWV6`, das bereits extern routbar ist. Ein eigenes Netz mit
Router und Floating IPs brächte nur eine NAT-Schicht ohne Nutzen für die Anwendung. Dafür setzt
der Aufbau voraus, dass die Default-Security-Group die Ports aus `### Voraussetzungen` öffnet
(siehe `## Grenzen`).

*Nachweis im Betrieb:* Der komplette Aufbau unter `## Betrieb` braucht außer den Zugängen aus
Phase 1 keinen Schritt im Portal. Nach `terraform apply` meldet ein erneutes `terraform plan`
`No changes`, und `infra/ansible/inventory/hosts.ini` ist aus den Terraform-Outputs erzeugt, nicht
von Hand gepflegt. `ansible-playbook playbooks/site.yml` endet erst, wenn beide Stufen der
ApplicationSets grün sind, danach zeigt `kubectl get nodes -L topology.ticketflow/region` vier
Nodes mit Regions-Label auf den Workern und `kubectl -n argocd get applications` alle 21
Applications *Synced/Healthy*. `terraform destroy` und ein erneuter Lauf beider Befehle bauen
denselben Stand wieder auf (siehe `## Betrieb`, Abbau und erneuter Aufbau).

**P3: Deployment und Konfiguration.**

*Wahl:* Helm-Werte legen Requests/Limits je Service fest
(z. B. `notification-worker`: 25m/100m CPU und 32Mi/64Mi Memory). Das ist bewusst klein, weil der
Dienst nur Kafka-Nachrichten konsumiert und kurze Log-/SMTP-Aufrufe macht, also anders als der
`reservation-worker` keine CPU-intensive Verarbeitung (siehe
[`charts/notification-worker/values.yaml`](charts/notification-worker/values.yaml)).
Images werden mit dem Git-SHA statt `:latest` getaggt, DB-Zugangsdaten/Kafka-Bootstrap-Server
kommen aus ConfigMap/Secret.

*Alternative 1 (Image-Referenz):* Pinning per Digest (`image@sha256:…`) statt eines Git-SHA-Tags.
*Begründung:* Ein Digest ist unveränderlich, ein Tag könnte theoretisch neu gepusht werden. Der
Short-SHA ist dagegen lesbar und führt direkt zum Commit: Wer im Vorverkauf ein fehlerhaftes
Verhalten sieht, findet über `imageTag` in `cluster.yaml` den Code-Stand. Ein Rollback ist ein
`git revert` dieser einen Zeile. *Trade-off:* Wir verlassen uns darauf, dass die CI einen SHA-Tag
nie überschreibt. Das gilt, weil jeder Build einen neuen Commit-SHA hat. Die CI pusht zusätzlich
`:latest` als Komfort-Tag für lokales `docker run`. Kein Chart verwendet ihn, und
`values.schema.json` lehnt ihn ab.

*Alternative 2 (Konfiguration):* Die ConfigMap als Datei mounten und im laufenden Prozess neu
laden, statt sie per `envFrom` als Umgebungsvariablen zu lesen. *Begründung:* Hot-Reload
bräuchte in jedem Service einen Datei-Watcher. Zudem liefen während einer Änderung alte und neue
Konfiguration gleichzeitig, z. B. bei einem neuen Topic-Namen mitten im Flash-Sale. Mit `envFrom`
kommt eine geänderte Konfiguration nur über ein Rolling Update an, Pod für Pod und abgesichert
durch die Readiness-Probe. *Trade-off:* Jede Konfigurationsänderung bedeutet Pod-Neustarts.
Secrets im Image kommen nicht in Frage: Sie wären in der Registry lesbar und je Umgebung nicht
austauschbar.

*Requests/Limits:* Die eigenen Services sind bewusst *Burstable* (Request < Limit), z. B.
`reservation-worker` mit 25m/48Mi Request und 250m/128Mi Limit. Die Alternative wäre
*Guaranteed* (Request = Limit). Damit bekäme jedes der bis zu zehn KEDA-Replicas seine Spitzenlast
fest reserviert, und auf drei Workern fände der Scheduler dann früher keinen Platz mehr. Der
Trade-off: Unter gleichzeitiger Spitzenlast aller Pods kann ein Node beim Speicher überbucht
werden, und der Kernel beendet dann einen Pod per OOM-Kill. Bei einem zustandslosen Worker ist das
verkraftbar, denn Kafka liefert die unbestätigte Nachricht erneut aus (P5). Bei einem Broker wäre
es das nicht. Deshalb setzt Kafka beim Speicher Request = Limit (siehe P4) und lässt nur die CPU
burstfähig, weil eine gedrosselte CPU bremst, aber keinen Zustand verliert.

*Nachweis im Betrieb:* `kubectl -n ticketflow get pods -o
custom-columns=POD:.metadata.name,IMAGE:.spec.containers[*].image` zeigt für jeden eigenen
Service ein Image mit dem Git-SHA aus [`gitops/env/cluster.yaml`](gitops/env/cluster.yaml)
(`imageTag`), nirgends `:latest`. `kubectl -n ticketflow get deploy ticketflow-app-catalog-api -o
yaml` zeigt die gesetzten `resources` sowie `envFrom: configMapRef` und die `secretKeyRef`s
auf das von CNPG erzeugte Secret `postgres-postgres-cluster-app` und das von Strimzi erzeugte
Kafka-Secret. `kubectl -n ticketflow get configmap ticketflow-app-catalog-api -o yaml` zeigt
Kafka-Bootstrap-Server, Topics und DB-Host als reine Konfiguration ohne Passwörter.

*P3: Unterpunkt: Packaging mit Helm.*

- *Wahl:* ein Chart je Komponente, alle nach demselben verbindlichen Muster (siehe
  [`charts/README.md`](charts/README.md) ## Muster für Applikations-Charts) mit
  `values-dev.yaml`, `values-prod.yaml` und `values.schema.json`. Übergreifende Werte
  (Image-Registry/-Tag, DNS-Zone, Umgebung) stehen einmalig in
  [`gitops/env/cluster.yaml`](gitops/env/cluster.yaml) und werden von ArgoCD per Multi-Source
  `$values` in jeden Chart gereicht.
- *Alternative:* Kustomize mit Base+Overlays.
- *Begründung:* Kustomize kennt kein Werte-Schema. Ein vergessener oder falsch typisierter Wert
  (etwa eine String- statt Int-Replica-Zahl, oder ein leerer Image-Tag) fiele erst zur Laufzeit im
  Cluster auf. Genau das ist hier schon einmal passiert: `notification-worker` lag mit leerem
  `image.tag` im Repo und war ohne `--set` gar nicht deploybar. Mit Helm+`values.schema.json`
  schlägt `helm lint`/`helm template` vorher fehl, bevor eine falsch konfigurierte CNPG-`Cluster`-
  oder Strimzi-`Kafka`-Ressource überhaupt angewendet wird. Bei zustandsbehafteten Custom
  Resources ist das der relevantere Fehlerzeitpunkt. Für den Flash-Sale-Pfad zählt konkret, dass
  das Umschalten von `dev` auf `prod` (mehr Replicas über alle drei Regionen, engere
  PodDisruptionBudgets) eine einzige Zeile ist und nicht ein Dutzend über Charts verstreute Werte.
- *Nachweis im Betrieb:* `kubectl -n argocd get app ticketflow-app -o yaml` zeigt unter
  `spec.sources` den Chart aus `charts/ticketflow-app` mit den Values-Dateien `values.yaml`,
  `values-dev.yaml` und `$values/gitops/env/cluster.yaml`. `helm template` mit einem Image-Tag
  `latest` bricht an `values.schema.json` ab.

**P4: Scheduling und Platzierung.**

*Wahl:* Jeder Worker trägt ein Node-Label
`topology.ticketflow/region` (`region-a`/`region-b`/`region-c`). Terraform vergibt es als Metadata, Ansible übersetzt es in ein
Kubernetes-Label. Die DHBWCloud stellt real nur eine Region/AZ bereit, deshalb ist die Verteilung
für diese Entscheidung bewusst simuliert. CNPG-Postgres (Primary + zwei Replicas,
`podAntiAffinityType: required`) und Kafka (Strimzi, je 3 Broker und 3 KRaft-Controller, auch im
aktiven dev-Profil, siehe [`charts/kafka-cluster/values-dev.yaml`](charts/kafka-cluster/values-dev.yaml))
erhalten je eine `topologySpreadConstraint` bzw. `podAntiAffinity` auf diesem Label, sodass ihre
Replicas nie auf demselben simulierten Standort landen. Für Kafka ist das eine **harte** Regel
(`whenUnsatisfiable: DoNotSchedule`,
[`kafka-broker-nodepool.yaml`](charts/kafka-cluster/templates/messaging/kafka-broker-nodepool.yaml)):
Das Topic `reservation-requested` ist der Puffer aller Flash-Sale-Anfragen, jede Partition liegt
dreifach vor und verlangt zwei bestätigte Kopien (`min.insync.replicas: 2`). Lägen zwei Broker in
derselben Region, würde deren Ausfall genau diese Mehrheit mitnehmen und `catalog-api` müsste
Reservierungen abweisen. Requests/Limits sind je Chart hergeleitet statt geschätzt: Kafka-Broker
1Gi Memory (Request = Limit, weil ein OOM-Kill bei einem zustandsbehafteten Broker Log-Recovery und
Leader-Wechsel auslöst) bei explizit 512m JVM-Heap, denn Kafka hält Nachrichten im OS-Page-Cache, nicht
im Heap. `reservation-worker` fordert nur 25m CPU/48Mi an, weil er I/O-gebunden auf Postgres wartet
und KEDA bis zu zehn Kopien gleichzeitig einplant. Die Rechnung steht jeweils als Kommentar in
[`charts/kafka-cluster/values.yaml`](charts/kafka-cluster/values.yaml) und
[`charts/reservation-worker/values.yaml`](charts/reservation-worker/values.yaml).

Dieselbe Regel gilt für alles, was im Ausfall eines Workers
nicht verschwinden darf. Jeder Dienst läuft mit **mindestens zwei Replicas, hart auf zwei
verschiedene Regionen verteilt** (`whenUnsatisfiable: DoNotSchedule`, `maxSkew: 1`, bzw. eine
`required`-`podAntiAffinity`, wo das Upstream-Chart kein `topologySpreadConstraints`-Feld hat, z. B.
AKHQ und Longhorn-UI). Das gilt für die eigenen Services (`catalog-api`, `frontend`,
`notification-worker`, `reservation-worker` mit KEDA-`minReplicaCount: 2`), AKHQ, Apicurio, Traefik
und die Operatoren (cert-manager, CNPG, Strimzi, KEDA, External Secrets, jeweils mit Leader-Election, wo
nötig). Jeder dieser Dienste hat ein PodDisruptionBudget. Die Regel ist hart statt
weich, weil eine weiche Präferenz genau im Engpass nachgibt:
Ist eine Region voll, legt `ScheduleAnyway` das zweite Replica neben das erste und der nächste
Node-Ausfall nimmt beide mit, mitten im Vorverkauf. `matchLabelKeys: [pod-template-hash]` sorgt
dafür, dass beim Rolling Update nur Pods derselben Revision zählen. Die Control-Plane hat kein
Regions-Label und einen `NoSchedule`-Taint ([`node_labels`](infra/ansible/roles/node_labels/tasks/main.yml))
und fällt damit für alle Workloads heraus. Welche Pods bewusst einzeln bleiben, steht unter
`## Grenzen`.

Auch die Requests/Limits der eigenen Services sind hergeleitet: `catalog-api` hat 50m/64Mi Request
(I/O-gebunden zwischen DB-Lesen und Kafka-Produce, kaum CPU im Leerlauf) und 300m/128Mi Limit als
Kopfraum für die JSON-Schema-Validierung vor jedem Kafka-Publish. `frontend` hat nur 10m/16Mi
Request (liefert nur statische Dateien aus) und 100m/64Mi Limit für viele gleichzeitige
Verbindungen beim Vorverkaufsstart. Die Begründung ist jeweils als Kommentar in
[`charts/ticketflow-app/values.yaml`](charts/ticketflow-app/values.yaml).

*Alternative:* freie Platzierung durch den
Standard-Scheduler, oder ein einzelner dedizierter "Storage-Node" für beide Dienste.

*Begründung:* Freie Platzierung könnte alle Replicas eines Dienstes zufällig auf denselben Node
verteilen. Der Ausfall genau dieses einen Nodes würde Ticket-Reservierungen und deren Event-Log
komplett verschwinden lassen. Ein einzelner Storage-Node wäre selbst wieder ein Single Point of
Failure und hätte die Platzierungsfrage nur verschoben. Sie soll aber aus der Domäne heraus beantwortet
werden: Entscheidend ist die Ausfallsicherheit beim Ticketverkauf, nicht ein Beispiel aus der
Vorlesung.

*Warum ein Regions-Label und nicht `kubernetes.io/hostname`:* Heute gibt es genau einen Worker pro
Region, beide Schlüssel würden also dieselbe Verteilung ergeben. Sie unterscheiden sich, sobald
ein Worker dazukommt. Ein Ticketanbieter skaliert vor einem großen Vorverkauf die Nodes hoch,
nicht die Regionen. Mit einer Regel auf den Hostnamen dürften dann zwei Kafka-Broker auf zwei
Nodes derselben Region landen, und ein Ausfall dieser Region (Strom, Rack, Uplink) nähme beide und
damit die `min.insync.replicas`-Mehrheit mit. Das Label modelliert den Fehlerbereich, nicht die
Maschine. Ein weiterer Worker bekommt in `worker_regions` einfach eine bestehende Region, und alle
Regeln gelten unverändert weiter. *Trade-off:* Die Regionen sind auf der DHBWCloud simuliert, die
Regel schützt hier real nur vor dem Ausfall einzelner VMs. Außerdem macht `DoNotSchedule` bei nur
drei Regionen eine Region zum Engpass: Ist ein Worker voll, bleiben neue Replicas `Pending`, statt
auf einen anderen Worker auszuweichen.

*Nachweis im Betrieb:* `kubectl get nodes -L topology.ticketflow/region` neben
`kubectl -n kafka get pods -l strimzi.io/cluster=ticketflow-kafka -o wide` zeigt je einen Broker und
Controller pro Region, `kubectl -n ticketflow get pods -l app.kubernetes.io/component=catalog-api -o wide`
(bzw. `=frontend`) die Verteilung der App-Pods über die Regionen. Die vollständige Verteilung
steht als Diagramm unter `## Architektur`.

**P5: Resilienz und Persistenz.**

*Wahl:* Liveness/Readiness-Probes auf allen eigenen Services, CNPG
verwaltet PVCs für Primary + zwei Replicas, Strimzi-Kafka-Broker laufen als StatefulSet mit PVC,
PodDisruptionBudgets sichern Rolling Updates und Node-Drains für jeden Dienst mit mindestens zwei
Replicas ab (z. B.
[`charts/notification-worker/templates/poddisruptionbudget.yaml`](charts/notification-worker/templates/poddisruptionbudget.yaml)).
Kafka: Replication-Factor 3 mit `min.insync.replicas: 2` und ein Strimzi-PDB `maxUnavailable: 1`.
Strimzi rollt Broker einzeln, und solange nur einer fehlt, nehmen die beiden anderen weiter
Reservierungen an. PVCs mit `deleteClaim: false` überleben jeden Pod-Neustart.
`reservation-worker` bestätigt den Kafka-Offset erst nach dem DB-Commit und bucht idempotent auf
`reservation_id`: Stirbt ein Pod mitten in der Verarbeitung, liefert Kafka die Nachricht erneut aus,
ohne dass ein Sitzplatz doppelt vergeben wird. Sein PDB ist `maxUnavailable: 1` statt eines
festen `minAvailable`, weil KEDA die Replica-Zahl zwischen 2 und 10 verändert.

*Alternative:* Ausfallsicherheit auf Speicherebene statt in der Datenbank. Eine einzelne
Postgres-Instanz läge auf einem Longhorn-Volume mit drei Replicas (`numberOfReplicas: 3`). Fällt ihr
Node aus, startet Kubernetes den Pod auf einem anderen Node neu, und Longhorn hängt dort eine
intakte Kopie des Volumes ein.

*Begründung:* Auch damit gingen keine Daten verloren. Der Ticketverkauf stünde aber still, bis der
Ausfall erkannt ist (Node `NotReady` und Pod-Eviction, mit Kubernetes-Standardwerten mehrere
Minuten), das Volume umgehängt ist und Postgres per Crash-Recovery wieder hochgefahren ist. Im
Vorverkauf sind ein paar Minuten ohne Buchung genau der Zeitraum, in dem fast alle Plätze vergeben
werden. Mit CNPGs Streaming-Replikation steht auf zwei anderen Nodes bereits eine laufende
Replica. Der Failover ist eine Beförderung, kein Neustart, und `catalog-api` liest währenddessen
weiter von der verbleibenden Replica. *Trade-off:* drei Postgres-Pods mit je 2Gi Memory-Request
statt einem. Außerdem ist die Replikation asynchron. Ein Commit, der beim ungeplanten Ausfall
des Primary noch nicht auf der Replica angekommen war, geht verloren, und zwar typischerweise nur die
letzten Millisekunden (siehe `## Grenzen`). Bei einem geplanten Switchover (Rolling Update von
Postgres) wartet CNPG, bis die Replica aufgeholt hat, dort tritt das nicht auf. Synchrone
Replikation würde die Lücke schließen, aber jede Buchung um einen Netzwerk-Roundtrip verlängern,
und das genau am Engpass des Flash-Sales. Longhorn bleibt bei einer Replica
(siehe `## Grenzen`), denn eine zweite Replikationsebene unter einer schon replizierten Datenbank
würde jeden Write ein weiteres Mal über das Netz schicken.

*Nachweis im Betrieb:*
1. **Pod-Verlust ohne Datenverlust:** eine Reservierung anlegen, bis sie `confirmed` ist. Dann
   `kubectl -n ticketflow delete pod -l app.kubernetes.io/name=reservation-worker` und den
   Postgres-Primary löschen (`kubectl -n ticketflow get cluster postgres-postgres-cluster` zeigt
   ihn in der Spalte `PRIMARY`, dann `kubectl -n ticketflow delete pod <primary>`). CNPG befördert
   eine Replica zum neuen Primary und die Spalte `PRIMARY` wechselt. Die Deployments starten neue
   Pods. `GET /reservations/{id}` liefert danach weiter `confirmed`, und der Sitzplatz bleibt in
   der Seat-Map belegt.
2. **Rolling Update ohne Ausfall:** während `src/load-generator/run.sh` läuft,
   `kubectl -n ticketflow rollout restart deploy ticketflow-app-catalog-api`. Bei zwei Replicas
   startet die Standard-Strategie `RollingUpdate` erst einen zusätzlichen Pod (`maxSurge` 25 %,
   aufgerundet 1) und nimmt keinen alten weg, bevor der neue bereit ist (`maxUnavailable` 25 %,
   abgerundet 0). Die Readiness-Probe lässt neue Pods erst in den Service, wenn DB und Kafka
   antworten. Die k6-Zusammenfassung zeigt deshalb nur `202`/`409` und keine Verbindungsfehler
   oder `5xx`. Das PDB (`minAvailable: 1`) sichert dasselbe für Node-Drains ab.
3. **Kafka-Broker-Verlust:** einen Broker-Pod löschen (`kubectl -n kafka get pods -l
   strimzi.io/pool-name=broker`, dann `kubectl -n kafka delete pod <broker>`). Mit RF 3
   und `min.insync.replicas: 2` nehmen die beiden anderen Broker weiter an, neue Reservierungen
   laufen ohne Fehler durch. Strimzi startet den Broker mit seinem bestehenden PVC neu.

**W2: GitOps (gewähltes Wahlthema).**

*Wahl:* ArgoCD mit zwei gestuften `ApplicationSets` (je eine Matrix aus
git-files- und list-Generator, zuerst die Plattform und die Anwendung erst, wenn die Plattform vollständig
Synced/Healthy ist), Auto-Sync und Self-Heal.

*Alternative:* Push-basiertes Deployment aus der CI. Der GitHub-Actions-Workflow, der ohnehin die
Images baut, würde nach dem Build `helm upgrade` gegen den Cluster ausführen, ohne einen Agenten im
Cluster.

*Begründung:* Ein Vorverkauf wird vorab vorbereitet: Profil `prod`, Replica-Zahlen, KEDA-Grenzen.
Bis zum Verkaufsstart muss der Cluster genau diesen geprüften Stand haben. Eine Push-Pipeline
prüft das nur in dem Moment, in dem sie läuft. Eine Änderung danach, z. B. eine während der
Vorbereitung per `kubectl` heruntergesetzte Replica-Zahl, bliebe unbemerkt bis zum nächsten Push,
im schlimmsten Fall bis in die Lastspitze. ArgoCD vergleicht dagegen laufend und dreht solche
Abweichungen per Self-Heal zurück. Hinzu kommt: Push-Deployment bräuchte eine Kubeconfig mit
Cluster-Admin-Rechten als GitHub-Secret. ArgoCD zieht dagegen von innen und braucht nur einen
Read-only-Deploy-Key. Ein kompromittiertes GitHub-Konto kann den Cluster so nur über einen
sichtbaren Commit verändern. *Trade-off:* Self-Heal
gilt auch im Notfall. Wer mitten im Flash-Sale per `kubectl` eingreift, wird von ArgoCD
zurückgesetzt, jede Notmaßnahme muss also über einen Commit laufen. Die einzige Ausnahme ist die
Replica-Zahl des `reservation-worker`. Die regelt KEDA, deshalb ignoriert ArgoCD sie
(`ignoreDifferences`, siehe B2).

Ansible bootstrappt nur so viel, dass ArgoCD läuft. Alles danach (Operatoren, Kafka, Postgres,
App) kommt aus Git. ApplicationSets statt einzelner `Application`-Dateien (App-of-Apps) sind
bewusst gewählt: Vorher stand die Repo-URL 20-mal im Repo, und ein Fork, der eine Stelle übersah,
bekam ein Deployment, das still gegen das Original-Repo lief. Die Zweiteilung kommt aus dem
Lastpfad: `catalog-api` und `reservation-worker` melden sich erst bereit, wenn Postgres, Kafka und
die Schema-Registry antworten, und die brauchen ihrerseits StorageClass, Operatoren und Secrets aus
dem Vault. Deshalb startet die Anwendung erst, wenn diese Plattform nachweislich grün ist. So läuft
ein frischer Cluster nicht minutenlang in Fehlschleifen, die man von echten Fehlern nicht
unterscheiden kann. ArgoCD-Sync-Waves helfen hier nicht, weil sie nicht auf von einem
ApplicationSet erzeugte Applications wirken. Progressive Syncs würden Self-Heal abschalten (siehe
[`gitops/README.md`](gitops/README.md) ## Gestufter Rollout).

*Nachweis im Betrieb:* Drift am Cluster vorbei erzeugen, z. B. `kubectl -n ticketflow scale
deploy ticketflow-app-catalog-api --replicas=5`. ArgoCD erkennt die Abweichung zum Git-Stand und
setzt per Self-Heal wieder auf 2 Replicas zurück (`kubectl -n ticketflow get deploy -w`,
`kubectl -n argocd get app ticketflow-app`). Umgekehrt landet eine Änderung erst per Commit+Push
im Cluster. Beim frischen Aufbau zeigt `gitops_rollout`, wie die Anwendungs-Stufe erst nach der
grünen Plattform-Stufe angewendet wird.

**Übernommener Code.** Folgende Teile stammen nicht von uns, sondern wurden übernommen und
angepasst:

- Die Konfiguration der Cluster-Addons Longhorn, cert-manager und external-dns ist portiert aus
  [`pfisterer/k3s-dhbw-cloud-role`](https://github.com/pfisterer/k3s-dhbw-cloud-role). Dort per
  Ansible-Rolle installiert, hier als Wrapper-Charts über ArgoCD (siehe
  [`gitops/README.md`](gitops/README.md) ## Privates Repo).
- Die JMX-Exporter-Regeln in
  [`kafka-metrics-configmap.yaml`](charts/kafka-cluster/templates/messaging/kafka-metrics-configmap.yaml)
  folgen dem Metrics-Beispiel aus den Strimzi-Examples (`examples/metrics/kafka-metrics.yaml`).
- Die AKHQ-RBAC-Rollen in [`charts/akhq/values.yaml`](charts/akhq/values.yaml) basieren auf AKHQs
  `application.example.yml`.
- Die NetworkPolicies für CNPG und Strimzi folgen den Vorgaben aus deren offizieller Doku
  (CNPG `samples/networkpolicy-example.yaml`, Strimzi „Restricting access to listeners with
  network policies“), siehe [`charts/README.md`](charts/README.md) ## network-policies.

## Grenzen

- `catalog-api` und `reservation-worker` teilen sich eine Datenbank und einen DB-User. Schema-
  Änderungen müssen rückwärtskompatibel erfolgen, der eine Postgres-Primary begrenzt den
  Buchungsdurchsatz und `catalog-api` liest von den Replicas. Erst wenn beide Replicas gleichzeitig
  fehlen, ist die API nicht bereit (Abwägung siehe `## Entscheidungen`, P1: Gemeinsame
  Datenbank).
- **Gemessen und erwartet unter Last.** Der im Screencast gezeigte Lauf (`run.sh`, 1000 virtuelle
  Nutzer, 75 s, dev-Profil) erzeugte rund 381.000 Reservierungsanfragen, also etwa 5.000 pro
  Sekunde. 11,7 % wurden mit `202` angenommen, 88,3 % mit `409` abgewiesen, weil der Platz schon
  vergeben war. Es gab keine `5xx` und keine Timeouts, p95 lag bei 389 ms, p99 bei 502 ms. KEDA
  skalierte den `reservation-worker` dabei von 2 auf 10 Pods. Die Last kam von einem einzigen
  Client über VPN, der Wert ist also eher eine Untergrenze dessen, was der Cluster annimmt, und
  keine gemessene Obergrenze. Bei echter, deutlich höherer Last erwarten wir den Engpass nicht in
  der API, sondern in der Verarbeitung. Mehr als 10 Worker-Pods (`maxReplicaCount`, 2 Partitionen
  je Pod) gibt es nicht, und alle schreiben auf den einen Postgres-Primary. Die Annahme bleibt
  schnell, weil Kafka puffert. Die Zeit bis zur Bestätigung `confirmed`/`rejected` wächst aber
  mit dem Rückstand. Weiter skalieren ließe sich nur über mehr Partitionen und eine Aufteilung der
  Sitzplätze auf mehrere Datenbanken (z. B. je Event), nicht über mehr Pods.
- **Datenverlust beim ungeplanten Failover.** Die Postgres-Replikation ist asynchron. Stirbt der
  Primary, bevor eine bereits bestätigte Buchung auf der Replica angekommen ist, fehlt sie nach
  dem Failover in der Datenbank. Den Kafka-Offset hat der Worker aber schon bestätigt, und das
  Ergebnis-Event ist schon versendet. Das Fenster liegt bei normaler Replikation im
  Millisekundenbereich, ist aber nicht null. Abhilfe wäre synchrone Replikation in CNPG, auf
  Kosten der Latenz jeder Buchung (siehe P5).
- **Security Group nicht im Code.** Terraform nutzt die Default-Security-Group des Projekts und
  verwaltet keine eigene. Sie muss die Ports aus `### Voraussetzungen` bereits erlauben. Ein
  Projekt mit restriktiverer Default-SG braucht dort eine Anpassung, die nicht aus dem Repo kommt.
- Die DHBWCloud stellt nur eine begrenzte Anzahl VMs/Ressourcen bereit. Daher sind CNPG- und
  Kafka-Replikation auf drei Kopien in drei simulierten Regionen ausgelegt, nicht auf mehrere
  echte Rechenzentren.
- Zahlungsabwicklung ist simuliert (kein echtes Payment-Gateway). `notification-worker` kann
  optional per SMTP echte E-Mails versenden, fällt aber standardmäßig auf reine Log-Ausgabe zurück (im
  Demo-Betrieb ist kein SMTP-Relay konfiguriert) und versendet keine echten E-Mails oder SMS.
- k3s statt eines vollwertigen Kubernetes verkleinert den Kontrollebenen-Fußabdruck, verhält sich
  aber nicht in jedem Detail wie ein Cloud-Provider-Kubernetes.
- Der Lastgenerator bildet einen synthetischen "alle klicken gleichzeitig"-Fall nach. Reales
  Nutzerverhalten (Netzwerklatenz-Verteilung, Retry-Verhalten von Browsern) ist nicht modelliert.
- Longhorn läuft mit einer Volume-Replica (`charts/longhorn/values.yaml`, `defaultReplicaCount: 1`).
  Ein **Pod**-Verlust ist damit unkritisch, ein **Node**-Verlust macht dessen PVCs aber unerreichbar,
  bis der Node zurück ist. Für Kafka fängt die Replikation auf Anwendungsebene (Replication-Factor 3
  über drei Regionen) das ab. Einzelne Volumes sind dadurch nicht zusätzlich gesichert.
- Nicht jeder Container im Cluster hat CPU- und Memory-Requests/-Limits. Alle eigenen Services,
  Kafka, Postgres und OpenBao haben sie. Ohne sie laufen nur Komponenten, deren
  Upstream-Chart bzw. Installer kein Feld dafür anbietet: Longhorns `longhorn-ui`,
  `longhorn-driver-deployer`, Engine-Image-DaemonSet und Instance-Manager (nur CPU-Request über
  `guaranteedInstanceManagerCPU`), ArgoCD (von Ansible als unverändertes Upstream-Manifest
  installiert) und k3s' eigene `kube-system`-Pods (`svclb-traefik`, `local-path-provisioner`,
  Teil-Limits bei CoreDNS/metrics-server). Alle liegen außerhalb des Flash-Sale-Lastpfads und
  verbrauchen im Betrieb wenige MiB. Ein LimitRange als Auffangnetz wurde bewusst nicht gesetzt,
  weil er für Longhorns privilegierte Pods geratene Werte erzwingen würde.
- Nicht jeder Pod läuft doppelt. Bewusst einzeln bleiben: `external-dns` (zwei Instanzen würden ohne Leader-Election
  dieselben DNS-Records gegeneinander schreiben, und ein Ausfall verzögert nur
  neue Records), Strimzis Entity-Operator und Kafka-Exporter (Strimzi unterstützt je nur eine
  Instanz), `longhorn-driver-deployer` (einmaliger Installer). Longhorns CSI-Sidecars laufen mit je
  drei Replicas, ihre Platzierung legt Longhorn selbst fest. ArgoCD ist als nicht-HA-Upstream-
  Manifest installiert (eine Instanz je Komponente), CoreDNS/metrics-server/local-path-provisioner
  verwaltet k3s selbst. Fällt ein Worker aus, stockt dort also höchstens Verwaltung (Deployments,
  Zertifikate, DNS), nicht der Ticketverkauf.
- Ein Node-Ausfall der Control-Plane ist nicht automatisch behoben (keine HA-Control-Plane geplant).
  Das ist eine bewusste Kosten/Nutzen-Entscheidung für den Laborrahmen.
- Alle Applications laufen im ArgoCD-Projekt `default`. Ein eigenes `AppProject`, das erlaubte
  Quell-Repos, Ziel-Namespaces und Ressourcentypen einschränkt, gibt es nicht. Wer Schreibzugriff
  auf das Repo hat, kann damit beliebige Ressourcen im Cluster ausrollen.


## Bonus

**B2: Autoscaling: KEDA auf Kafka-Consumer-Lag.**

*Was umgesetzt wurde:* KEDA (`charts/keda`,
Operator, `ScaledObject`/`TriggerAuthentication` in
`charts/reservation-worker/templates/keda-scaledobject.yaml`) skaliert `reservation-worker` auf
Basis des Kafka-Consumer-Group-Lags von `reservation-requested`, nicht auf CPU-Auslastung. Die
eigentliche fachliche Engpassgröße ist "wie viele Reservierungen warten auf Verarbeitung", nicht
CPU-Last, die bei I/O-gebundener Arbeit (Warten auf DB-Commit, Kafka-Fetch) trügerisch niedrig
bleiben kann, obwohl die Warteschlange wächst und Kunden auf ihre Bestätigung warten.

| Wert | dev (aktuell synct) | prod | Begründung |
|---|---|---|---|
| Partitionen `reservation-requested` | 20 | 20 | Obergrenze paralleler Consumer je Consumer-Group. Nicht je Profil verschieden: Kafka kann Partitionen nie reduzieren |
| `keda.minReplicaCount` / `maxReplicaCount` | 2 / 10 | 2 / 10 | min. 2: auch ohne Last zwei Pods in zwei Regionen (P4). Max. 10 statt 20: jeder Pod bekommt genau 2 Partitionen. Die eigentliche Grenze dahinter ist der eine Postgres-Primary, auf den alle Pods schreiben. 20 Partitionen lassen Reserve, ohne das Topic neu anzulegen |
| `keda.lagThreshold` | 10 | 10 | Ziel-Rückstand je Pod, gewünschte Replicas = ⌈Gesamt-Lag / 10⌉. Bewusst niedrig gegenüber den 1000 Plätzen je Halle (`src/reservation-worker/internal/migrate/sql/0002_seed_demo_events.sql`): skaliert, sobald der Zufluss die Verarbeitung eines Pods übersteigt |
| `keda.pollingInterval` | 15s | 15s | wirkt bei `minReplicaCount` ≥ 1 nicht: dann fragt das HPA den Lag selbst alle 15s über KEDAs Metrics-API ab und verdoppelt je 15s (bzw. +4 Pods), 2 → 10 in ~30s. Gesetzt bleibt er nur für den Fall einer Skalierung auf 0 |
| `scaleDownStabilizationSeconds` | 60s | 60s | Rückskalieren nach 60s statt nach dem HPA-Default von 300s. `cooldownPeriod` wirkt nur beim Skalieren auf 0, bei `minReplicaCount` ≥ 1 also gar nicht |

Der `replicaCount` von `reservation-worker` im Chart ist seit KEDA nur noch der Deployment-Startwert. Das
Element `reservation-worker` im ApplicationSet ignoriert `spec.replicas` explizit
(`ignoreDifferences`), sonst würde ArgoCDs `selfHeal` jede KEDA-Skalierung sofort wieder
zurücksetzen. Dies ist ein Standardmuster für ArgoCD zusammen mit einem HPA/KEDA-verwalteten Deployment.

*Alternative:* HPA auf CPU-Auslastung.

*Begründung:* Die Lag-Metrik kostet einen zusätzlichen Scaler (KEDA), während die HPA-CPU-Metrik im
Cluster bereits eingebaut ist. Der Mehraufwand lohnt sich hier, weil eine CPU-Skalierung im Lasttest
am falschen Signal ansetzen würde.

*Nachweis im Betrieb:* `src/load-generator/run.sh` erzeugt den Backlog.
`kubectl get scaledobject,hpa -n ticketflow` während des Lasttests zeigt die wachsende
Replica-Zahl. Da es kein Grafana-Dashboard gibt, zeigt AKHQ die Wirkung stattdessen live über
den wachsenden und wieder fallenden Consumer-Lag.

**B3: Weiteres Wahlthema: W7 Cluster-Härtung.**

*Was umgesetzt wurde (NetworkPolicy-Hälfte):* Jeder
Namespace startet mit **default-deny für Ingress und Egress**. Danach ist jede einzelne Verbindung explizit
erlaubt, Komponente für Komponente (`charts/network-policies`, ~80 Policies, eine
Verkehrsmatrix an einer Stelle). Beispiele für die Enge: der anonyme Kafka-Listener 9093 nimmt
nur noch Apicurio an, der SCRAM-Listener 9092 nur die vier `KafkaUser`-Clients plus KEDAs
Lag-Scaler. Die App-Postgres nimmt nur `catalog-api`/`reservation-worker`, den CNPG-Operator und die
eigenen Replikas an. Internet-Egress haben nur cert-manager (ACME/DNS-01), CoreDNS (Upstream-DNS),
der ArgoCD-Repo-Server (GitHub/Helm-Repos) und ein Hook-Job, jeweils auf die nötigen Ports
beschränkt. Die Upstream-Policies einzelner Charts sind abgeschaltet. Sie waren teilweise zu offen
(AKHQ, Strimzi-Listener „von allen Namespaces“).

*Alternative:* die Upstream-Schalter der einzelnen Charts einschalten und nur Lücken selbst
schließen, oder ein CNI mit L7-Policies (Cilium) statt k3s' eingebautem kube-router.

*Begründung:* Upstream-Policies sind pro Chart unterschiedlich weit. CNPG, Longhorn und
external-dns bringen gar keine mit, KEDAs sperrt auf k3s den API-Server aus. Ohne default-deny ist
das Ergebnis ein Flickenteppich, bei dem jeder nicht erfasste Pod offen bleibt. Für die Domäne zählt genau das: Im Flash-Sale ist `catalog-api` der am stärksten
exponierte Dienst. Selbst kompromittiert darf er Postgres nur auf Port 5432 lesen, Kafka nur über seinen
eigenen SCRAM-User erreichen und weder OpenBao noch den anonymen Kafka-Listener sehen. Cilium
hätte einen CNI-Wechsel des bestehenden k3s-Clusters bedeutet. Standard-NetworkPolicies reichen für
L3/L4 und bleiben mit jedem CNI portabel. Die Matrix wurde nicht geraten, sondern aus dem
Live-Cluster abgeleitet (Webhooks, APIServices, Monitor-Ziele, Ingress-Backends, per `ss`
mitgeschnittene offene Verbindungen) und vor dem Rollout gegen alle diese Flows simuliert, siehe
`charts/README.md` ## network-policies.

*Was umgesetzt wurde (Secret-Vault-Hälfte):* Applikations-Secrets, die vorher chart-eigen/hardcodiert im
Repo lagen (AKHQs Kafka-SASL-Passwort, AKHQs Basic-Auth-Passwort-Hash und JWT-Schlüssel,
`notification-worker`s SMTP-/Kafka-mTLS-Platzhalter), verwaltet jetzt **OpenBao**, ein Fork von
HashiCorp Vault (`charts/openbao`). OpenBao läuft hochverfügbar mit drei Replicas über die drei
simulierten Regionen (P4) und nutzt **PostgreSQL als Storage-Backend** (`charts/openbao-postgres`,
`ha_enabled = "true"`). Der **External Secrets Operator** (`charts/external-secrets-operator` und
`charts/openbao-secretstore`) synchronisiert die Werte als native Kubernetes-`Secrets`. Die Deployments lesen sie unverändert per
`secretKeyRef`/`envFrom`. Das Postgres-App-Credential steht nicht mehr im Repo: CNPG generiert
es selbst, `catalog-api`/`reservation-worker` lesen es direkt aus dem CNPG-Secret.

Initialisieren, Entsiegeln und das Befüllen der KV-Pfade übernimmt ein Helm-Hook-Job in
`charts/openbao` (`templates/bootstrap-job.yaml`), nach demselben Muster wie der
Schema-Registrierungs-Job in `charts/kafka-cluster`. Damit kommt OpenBao wie alles andere über
ArgoCD hoch, ohne zusätzliche Ansible-Rolle (P2, W2). Manuell bleibt nur die empfohlene einmalige
Sicherung von `openbao-unseal-keys` nach dem ersten Sync.

*Bewusste Vereinfachung:* Die Shamir-Schwelle ist `1-von-1` statt z. B. `3-von-5`. Eine echte
Schlüsselverteilung setzt mehrere unabhängige Verantwortliche voraus, die im Laborbetrieb fehlen.
Der Auto-Unseal-Sidecar fragt lokal `bao status` ab und entsiegelt bei Bedarf mit dem Schlüssel
aus einem gemounteten Secret. Der Preis: Unseal-Key und Root-Token liegen als Kubernetes-Secret im
selben Cluster. Wer Lesezugriff auf den Namespace `openbao` hat, hat damit auch Zugriff auf den
Vault. In Produktion gehörten der Unseal-Key in ein externes KMS und das Root-Token nach dem
Bootstrap widerrufen.

Secrets fließen auch in die umgekehrte Richtung: ein `PushSecret` (ESOs Gegenstück zu
`ExternalSecret`, K8s → Vault statt Vault → K8s) spiegelt die von Strimzi (alle vier
`KafkaUser`-Passwörter) und CNPG (Postgres-App-User) erzeugten Zugangsdaten zusätzlich nach
OpenBao (`updatePolicy: Replace`, die Kopie folgt dem Original), damit OpenBao wirklich *jedes*
Credential im Cluster als zentrale Audit-Fläche zeigt, nicht nur die selbst Erzeugten. Die
Dienste lesen trotzdem direkt aus den Operator-Secrets. Die einzige Ausnahme ist AKHQ, das sein
Kafka-Passwort aus einem anderen Namespace braucht (siehe B4). Bewusst ausgenommen bleibt OpenBaos
eigenes Storage-Backend-Credential (`charts/openbao-postgres`). Das wäre zirkulär: Der Vault kann
sein eigenes Bootstrap-Passwort nicht enthalten, bevor er damit entsiegelt ist.

*Alternative:* SOPS (age-Backend), verschlüsselte Dateien direkt im Repo eingecheckt, von Ansible
zur Laufzeit entschlüsselt, oder Sealed Secrets (Controller-basiert).

*Begründung:* SOPS ist bei einer überschaubaren Zahl an Secrets einfacher, aber statisch. Jede
Rotation ist ein Git-Commit, und der gesamte Chiffretext-Verlauf bleibt für jeden mit Repo-Zugriff
und dem age-Key entschlüsselbar, auch alte, längst rotierte Werte. Sealed Secrets bräuchte einen
zusätzlichen Controller nur für diesen einen Zweck und hätte dasselbe Rotationsproblem. Vor allem
decken beide nur Secrets ab, die *wir* erzeugen. Die meisten Zugangsdaten im Cluster erzeugen aber
Strimzi und CNPG zur Laufzeit, und die tauchen in keinem Repo auf. Nur ein Vault kann sie per
`PushSecret` an einer Stelle zusammenführen. *Trade-off:* Ein eigener Vault-Dienst ist deutlich
aufwendiger: drei zusätzliche Replicas, eine eigene Postgres-HA-Instanz und ein Bootstrap-Job.
Dafür gibt es Rotation ohne Git-Commit, eine zentrale Audit-Fläche und Hochverfügbarkeit über
dieselben simulierten Regionen wie bei Kafka und CNPG. Ein versiegelter Vault hält den
Ticketverkauf trotzdem nicht an, weil die Anwendung ihre Zugangsdaten direkt aus den
Operator-Secrets liest.

*Zugriff (Login):* Das Root-Token ist das einzige Anmeldemittel für Menschen. Es gibt bewusst
keine Benutzername/Passwort-Auth-Methode (userpass/LDAP), nur das Shamir-Root-Token und
Kubernetes-Auth (nur für ServiceAccounts, siehe oben). Auslesen und Anmelden:
```bash
export KUBECONFIG=infra/ansible/kubeconfig
ROOT_TOKEN=$(kubectl -n openbao get secret openbao-unseal-keys -o jsonpath='{.data.root_token}' | base64 -d)
echo "$ROOT_TOKEN"   # per CLI im Pod einloggen:
kubectl -n openbao exec -it openbao-0 -- bao login "$ROOT_TOKEN"
kubectl -n openbao exec -it openbao-0 -- bao kv get secret/postgres/app-user   # Beispiel-Secret
```
Alternativ über die Web-UI unter `https://openbao.<dnsZone>` (siehe `## Betrieb`, `dnsZone` aus
`gitops/env/cluster.yaml`) mit demselben Token einloggen (Auth-Methode "Token") und unter
"Secrets Engines" → `secret` durch die KV-v2-Pfade browsen (`akhq/basic-auth`, `kafka/<user>`,
`postgres/app-user`, `notification-worker/{smtp,kafka-tls}`). Kein separates Passwort für die UI
nötig/vorhanden. Wer das Root-Token nicht aus dem Cluster lesen kann, kommt so oder so nicht
hinein, ein öffentlicher Ingress vor der UI öffnet also keinen zusätzlichen Zugriffsweg (siehe
`charts/openbao/templates/ingress.yaml`).

*Nachweis im Betrieb:* `kubectl -n openbao get pods` zeigt 3 laufende, entsiegelte
OpenBao-Replicas. Ein simuliertes Pod-Kill der aktuell aktiven Instanz (`kubectl delete pod
openbao-X -n openbao`) zeigt Failover auf einen bereits entsiegelten Standby ohne manuelles
Eingreifen (Postgres-Backend-Leader-Wahl). Ein Neustart/Ersatz-Pod zeigt den Auto-Unseal-Sidecar
beim automatischen `bao operator unseal`, ohne einen erneuten manuellen Schritt. `kubectl -n
<app-namespace> get externalsecret` zeigt `SecretSynced`. `kubectl -n <app-namespace> get
pushsecret` zeigt `Synced` für die Strimzi-/CNPG-Credentials, `bao kv get secret/kafka/catalog-api`
bzw. `bao kv get secret/postgres/app-user` zeigt den gespiegelten Wert (`updatePolicy: Replace`:
die Kopie folgt dem Original, auch wenn ein Operator das Passwort neu erzeugt). Die Anwendung
selbst liest ihre Kafka- und Postgres-Zugangsdaten direkt aus den Operator-Secrets, nicht aus
OpenBao. Ein versiegelter Vault hält also den Ticketverkauf nicht an. NetworkPolicy-Hälfte:
`kubectl get networkpolicy -A` zeigt je Namespace `default-deny-all` + `allow-*`. Negative
Probe: ein Pod ohne passendes Label erreicht CNPG:5432 und den Kafka-Broker nicht.
```bash
kubectl -n ticketflow run np-probe --rm -it --restart=Never --image=busybox:1.36 -- \
  sh -c 'nc -zw3 postgres-postgres-cluster-rw 5432 || echo BLOCKIERT'
```

**B4: Neue Technologie: Kafka-Betrieb mit Strimzi über den Beispielumfang der Vorlesung
hinaus.**

*Was umgesetzt wurde:* Statt eines einzigen, cluster-weit geteilten Kafka-Credentials
für alle Services bekommt jeder Client über Strimzis **User Operator** eine eigene
`KafkaUser`-Custom-Resource mit SCRAM-SHA-512-Authentifizierung und ein eigenes, engst mögliches
`spec.authorization` (`type: simple`, ACLs), siehe `charts/kafka-cluster/templates/messaging/
kafka-users.yaml`, generiert aus `values.yaml` `kafka.users`: `catalog-api` darf nur `Write` auf
`reservation-requested`, kein `Read`. `reservation-worker` darf `Read` auf
`reservation-requested` (+ die eigene Consumer-Group) und `Write` auf `reservation-result`.
`notification-worker` darf ausschließlich `Read` auf `reservation-result` (+ eigene
Consumer-Group). Der Kafka-Cluster läuft in einem **eigenen Namespace** (`kafka`), getrennt von
der Anwendung (`ticketflow`). Für die Secrets folgt daraus, dass die `KafkaUser`-CRs aller
vier Identitäten (`catalog-api`/`reservation-worker`/`notification-worker`/`akhq`) bewusst
im **App**-Namespace angelegt werden (`spec.entityOperator.userOperator.watchedNamespace`), damit das vom
User Operator erzeugte Secret dort landet, wo `catalog-api`/`reservation-worker`/
`notification-worker` es direkt per `secretKeyRef` lesen können. Bei allen vier Identitäten generiert Strimzi das
Passwort selbst. **AKHQ** läuft aber im Kafka-Namespace und kann das App-Namespace-Secret
deshalb nicht per `secretKeyRef` erreichen (Kubernetes erlaubt das nicht über Namespace-Grenzen).
Ein `PushSecret`/`ExternalSecret`-Paar über OpenBao liefert genau das. AKHQs Kafka-Passwort wird wie bei den anderen
drei von Strimzi generiert, nach OpenBao gespiegelt und von dort in AKHQs eigenen Namespace zurückgesynct.

*Alternative:* ein einziges, geteiltes Client-Secret für alle
Producer/Consumer oder Kafka-ACLs händisch per
`kafka-acls.sh` statt deklarativ über `KafkaUser`.

*Begründung:* Ein geteiltes Secret ist
funktional ausreichend, um Nachrichten zu produzieren/konsumieren, macht aber jeden Service
implizit zu einem Producer/Consumer auf *jedem* Topic. Ein Bug in `notification-worker` könnte
dann unbemerkt in `reservation-result` schreiben und stille Dateninkonsistenz erzeugen, statt
sofort mit einer `TopicAuthorizationException` abgelehnt zu werden. Händische
`kafka-acls.sh`-Aufrufe wären nicht GitOps-versioniert und müssten bei jedem Cluster-Rebuild
manuell wiederholt werden. `KafkaUser` als Custom Resource kommt wie alles andere aus Git und wird
von ArgoCD mitverwaltet (siehe W2).

*Abgrenzung:* Für P1 genügt ein Kafka-Cluster, der Nachrichten transportiert. B4 beansprucht nur
das, was darüber hinausgeht: User Operator, SCRAM, ACLs je Client, die Trennung der Namespaces und
der ausdrücklich hart verteilte KRaft-Betrieb (P4). Die NetworkPolicies aus B3 begrenzen, *wer*
den Broker erreicht (L3/L4). Die ACLs hier begrenzen, *was* ein authentifizierter Client auf
welchem Topic tun darf. Es sind also zwei unabhängige Schutzebenen mit verschiedenen Technologien.

*Tragend, nicht dekorativ:* Die ACLs ändern das tatsächliche
Fehlerverhalten bei einem Bug (harter Autorisierungsfehler statt stiller Fehlschreibung) und sind
damit Teil des Arguments gegen Datenverlust und Doppelbuchungen aus P5, nicht nur eine
zusätzliche Konfigurationsdatei.

*Nachweis im Betrieb:* `kubectl get kafkauser -n ticketflow` zeigt die vier
Nutzer. Ein Test-Producer mit echten Credentials des `notification-worker` (ein `kubectl run`-Pod
mit dem Strimzi-Kafka-Image, der versucht, auf `reservation-requested` zu schreiben) wird von Kafka
mit `TopicAuthorizationException` abgelehnt. AKHQ zeigt die Consumer-Groups mit wachsendem/
fallendem Lag live während der Lastdemo (siehe B2: Autoscaling oben).
