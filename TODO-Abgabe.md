# Offene Punkte bis zur Abgabe

Abgleich des Repos mit der Aufgabenstellung V2 (`2026_07_23_Aufgabenstellung - v2.pdf`),
Stand 13.10.2026 (`main` @ `d78cad0` + README-Korrekturen vom 13.10.). Sortiert nach Auswirkung auf die Bewertung. Datei vor der
Abgabe löschen.

Legende: 🔴 blockiert Punkte oder macht Teile unbewertbar · 🟠 kostet wahrscheinlich Punkte ·
🟡 Feinschliff · ❓ Stand aus dem Repo nicht erkennbar, im Team klären

## Überblick

| Bereich | Punkte | Stand |
|---|---|---|
| Teil A – P1 Anwendung/Domäne | 8 | erfüllt |
| Teil A – P2 IaC, frisches Projekt | 15 | umgesetzt, Nachweis-Satz in README, ❓ **Test auf frischem Projekt** |
| Teil A – P3 Requests/Limits, Tags, Config | 10 | erfüllt |
| Teil A – P4 Platzierung + Nachweis | 10 | erfüllt, Diagramm vom 09.10. aus dem Live-Cluster |
| Teil A – P5 Resilienz | 12 | umgesetzt, Nachweis-Satz in README (noch nicht live geprüft) |
| Teil B – Wahlthema | 10 | W2 (GitOps), Nachweis-Satz in README vorhanden |
| Teil C – Doku/Darstellung | 35 | Screencast liegt vor, README am 10.10. final gelesen, Unter-READMEs am 13.10. bereinigt |
| Bonus | ≤ 10 | B2/B3 (W7)/B4 beansprucht; **B2 ohne Grafana-Dashboard** |

---

## 🔴 Kritisch

### 1. ~~Screencast fehlt komplett~~ ✅ erledigt (09.10.)

`screencast/Screencast.mp4` liegt im Repo (6:58 min, enthält einen Kapitel-Track),
`.git` insgesamt ~36 MB. `screencast/SCRIPT.md` und `screencast/README.md` wurden dabei gelöscht.

- [ ] ❓ Kurz gemeinsam gegenprüfen: spricht **jedes Gruppenmitglied**, sind die Kapitelmarken im
      Player sichtbar, ist die **Pflichtszene** (Pod löschen → Erholung → Daten noch da) drin?
- [ ] ❓ Sind die Szenen für W2 (Drift → ArgoCD dreht zurück), B3 (NetworkPolicy/OpenBao) und
      B2 (Last → Replicas wachsen) enthalten? Was fehlt, sollte im README nicht als
      „im Screencast gezeigt“ behauptet werden.

### 2. ~~Wahlthema festlegen~~ ✅ erledigt (05.10.)

W2 (GitOps) ist das gewertete Wahlthema, W7 (Cluster-Härtung) läuft als B3 im Bonus. W1 entfällt.

### 3. ~~README-Struktur: `## ToDo` muss raus~~ ✅ erledigt (04.10./09.10.)

`## ToDo` ist entfernt, der Status-Kasten über `## Problemstellung` ebenfalls.

### 4. B2 Autoscaling: Grafana-Dashboard wird im Screencast verlangt

Abschnitt 6.3: „Wenn Sie den Bonus B1 oder B2 beanspruchen: Last erzeugen, Replicas wachsen,
**Wirkung im Grafana-Dashboard zeigen**“. Es gibt weiterhin keinen Observability-Stack; die README
zeigt die Wirkung über `kubectl get scaledobject,hpa` und weist auf das fehlende Dashboard hin.

- [x] Verweise „kein Grafana-Dashboard, siehe `## Grenzen`“ entfernt; der fehlende Stack wird
      direkt an den Stellen (Architektur, Betrieb, B2) benannt, nicht in `## Grenzen`.
- [ ] Im Team bestätigen: Stack wird **nicht** wieder aufgebaut → geringere B2-Punkte bewusst in
      Kauf nehmen.

### 5. P2: kompletter Aufbau auf einem frischen Projekt testen (15 Punkte, sonst A+B unbewertbar)

„Wenn der Stack sich nicht aus dem Repository heraus aufbauen lässt, sind Teil A und Teil B nicht
bewertbar.“ Im Repo ist kein Neuaufbau seit dem 04.10. dokumentiert.

- [ ] ❓ Wurde seitdem auf einem frischen Projekt neu aufgebaut? Falls nein: nur mit den Schritten
      aus `## Betrieb` aufbauen, Abweichungen korrigieren.
- [ ] Prüfen, ob alles auf 3 Worker passt und `gitops_rollout` je Stufe in 30 min grün wird.

---

## 🟠 Wichtig

### 6. Nachweise im Betrieb in `## Entscheidungen` ergänzen

Für P1, P3 und P5 am 12.10., für P2 am 13.10. ergänzt, damit hat jetzt jedes Thema einen
„*Nachweis im Betrieb*“-Satz. Die Reihenfolge ist überall Wahl → Alternative → Begründung →
Nachweis. Die Kommandos sind nur gegen die Charts geprüft, **nicht live**.

- [ ] Am 13.10. gemeinsam am Cluster ausführen und README bei Abweichungen anpassen:
      **P5** (Worker- + Postgres-Primary-Pod löschen → Reservierung noch `confirmed`;
      `rollout restart` von catalog-api während `run.sh` → keine `5xx`; Broker-Pod löschen),
      **P1** (AKHQ-Consumer-Groups), **P3** (Image-Spalte, Deployment-/ConfigMap-YAML),
      **P2** (`terraform plan` nach dem Apply meldet `No changes`).

### 7. ~~Diagramme an den aktuellen Stand anpassen~~ ✅ erledigt (09.10.)

Beide Diagramme in `docs/architecture/` aus dem Live-Cluster neu erzeugt, mit Pod-Anzahlen.

### 8. `## Grenzen`: Verhalten unter echter Last konkret benennen

Der `TODO`-Hinweis ist aus der README entfernt, es stehen aber weiterhin **keine Messwerte** drin
(Vorgabe: „was Sie unter echter Last erwarten“).

- [ ] Werte aus einem Lasttest (`src/load-generator/run.sh`, seit 08.10. eine Minute Default)
      eintragen: Reservierungen/s, Lag-Verlauf, Zeit bis zur Skalierung, Postgres-Primary als
      Engpass. Naheliegend für Carlos (`reservation-worker`/KEDA).
- [ ] Optional B6 „Belastbare Messung“ (bis 5 P.) – bei Stand 12.10. eher nicht mehr.

### 9. Fremden Code in `## Entscheidungen` benennen (Abschnitt 9, Formales)

„Fremder Code ist erlaubt, wenn er in `## Entscheidungen` als solcher benannt wird.“ Am 13.10.
als Absatz **„Übernommener Code“** am Ende von `## Entscheidungen` ergänzt:

- [x] Addon-Konfiguration portiert aus `pfisterer/k3s-dhbw-cloud-role` (Longhorn, cert-manager,
      external-dns)
- [x] JMX-Exporter-Regeln aus dem Strimzi-Metrics-Beispiel
- [x] AKHQ-RBAC-Rollen aus AKHQs `application.example.yml`
- [x] NetworkPolicy-Vorgaben aus CNPG-/Strimzi-Doku
- [ ] Carlos bestätigt die Herkunft von JMX-Regeln und AKHQ-Rollen; Team fragt nach weiteren
      übernommenen Snippets

### 10. ~~Lokale Commits mit dem Remote zusammenführen und pushen~~ ✅ erledigt

Lokal und Remote sind auf demselben Stand, `gitops/env/cluster.yaml` steht auf `6fb800dd`.

- [ ] ❓ Live prüfen, dass alle Applications Synced/Healthy sind.

### 11. Zugang für den Prüfer

Abgabe als GitHub-Link verlangt „öffentlich oder Zugriff für Github-User `pfisterer`“; das Repo
ist privat.

- [ ] ❓ `pfisterer` als Collaborator (Lesezugriff) hinzugefügt? Alternativ ZIP per `git archive`
      (max. 100 MB, keine lokalen Artefakte wie `terraform.exe`/`.terraform/`).

---

## 🟡 Feinschliff

### 12. ~~Rechtschreibung und Konsistenz~~ ✅ erledigt (10.10.)

README final gelesen (Carlos, 10.10.), Bonus-Bezeichnungen vereinheitlicht (Philipp, 10.10.).

### 13. Nachvollziehbarkeit für Fremde (Teil C, 4 Punkte)

- [ ] `## Betrieb` einmal von jemandem durchgehen lassen, der das Projekt nicht kennt.
- [x] Manuelle Vorbedingungen gekennzeichnet: `## Betrieb` Phase 1 („lassen sich nicht aus dem
      Repo heraus automatisieren“) und Schritt 11 (ghcr.io-Packages auf „Public“).

### 14. Commit-Verlauf / Beiträge

Aktuelle Verteilung: Xaron 207, Carlos-Ku9 75, Philipp Stiegler 32, alea563-blipp 6 Commits.

- [ ] Sicherstellen, dass jedes Mitglied erkennbar ist (ggf. `.mailmap` für `Xaron`/
      `alea563-blipp` → Klarname).

### 15. Offene technische Punkte aus der Doku

- [x] ArgoCD-`AppProject`: nicht umgesetzt, `project: default` steht jetzt in `## Grenzen`.
- [ ] ❓ OpenBao-HA-Failover zwischen entsiegelten Standbys live getestet? (in B3 als Nachweis
      behauptet)
- [ ] ❓ B4-Negativprobe (falscher `KafkaUser` schreibt auf fremdes Topic → Autorisierungsfehler)
      tatsächlich ausgeführt bzw. im Screencast? Die Kommandos standen in `screencast/SCRIPT.md`,
      das am 09.10. gelöscht wurde (Carlos).

---

## Aus dem Team-ToDo der README übernommen

**Reviews durch die Verantwortlichen** – technisch umgesetzt, laut Team-ToDo noch zu prüfen:

- [ ] P3 – Niklas prüft (Requests/Limits, SHA-Tags, ConfigMap/Secret)
- [ ] P4 – jeder für sein Helm-Chart (harte Regions-Verteilung, ≥ 2 Replicas, hergeleitete
      Requests/Limits); Carlos: `kafka-cluster`, `apicurio`, `akhq`, `reservation-worker`
- [ ] P5 – jeder für sein Helm-Chart (Probes, PVC/StatefulSet, PDB, Pod-Verlust ohne Datenverlust)
- [ ] W2 – Niklas prüft, ob GitOps so passt

**Entscheidungen:**

- [x] Wahlthema – W2 gewertet, W7 als B3, siehe Punkt 2

**Screencast:**

- [x] „Alle zusammen“ – aufgenommen (09.10.), Gegenprüfung siehe Punkt 1

**Bonus-Ideen, nicht umgesetzt** (bei Stand 12.10. nicht mehr realistisch, nur zur Vollständigkeit):

- Observability mit OpenTelemetry (B1) – Stack entfernt, siehe Punkt 4
- Chaos-Experiment (B7)
- Helm-Charts veröffentlichen (B8)

### 16. ~~READMEs bereinigen~~ ✅ erledigt (13.10.)

- Verweise auf die gelöschte `CLAUDE.md` (7×) durch Inhalt bzw. gültige Verweise ersetzt
- Bonus-Nummern in Unter-READMEs, Chart-Kommentaren und ApplicationSets an die Root-README
  angeglichen (OpenBao/NetworkPolicies = B3, KafkaUser/ACLs = B4)
- Tote Querverweise („P1/P3 – Ergänzung“, `## GitOps secrets`), veraltete Migrationsabschnitte
  (App-of-Apps, Kafka 1→3 Broker), „P6“, „Screenshots folgen“, Lastgenerator-Dauer korrigiert
- ae/oe/ue-Schreibweisen und Grammatikfehler in der Root-README behoben

## Außerhalb dieser Aufgabenstellung

- `Laborbericht_teil_sturm.md` ist ein separater Prüfungsteil (zuletzt aktualisiert 06.10.);
  Stand dort separat prüfen.

## Bereits erfüllt (zur Orientierung)

- Zwei+ eigene Services (catalog-api, reservation-worker, notification-worker, frontend), über
  Kafka/HTTP verbunden; zustandsbehaftete Dienste Postgres (CNPG, 3 Instanzen) und Kafka (Strimzi)
- Künstlicher Lastpfad (`src/load-generator`, mit Latenz-Perzentilen und Durchsatz-Zusammenfassung)
- Images bauen per GitHub Actions aus `src/`, Tag = Git-SHA, keine `:latest`-Referenzen in Charts
- Terraform/OpenTofu + Ansible, ein Ablauf (`terraform apply` + `ansible-playbook site.yml`)
- Requests/Limits mit hergeleiteten Zahlen; Konfiguration über ConfigMap/Secret
- Harte, domänenbegründete Platzierung, alle Dienste ≥ 2 Replicas hart verteilt, Control-Plane
  getaintet
- Probes, StatefulSets/PVCs, PDBs, idempotente Verarbeitung, RF 3 / min.insync 2
- Keine externen Cloud-Dienste (nur ghcr.io als Registry und GitHub als Git-Hoster)
- Screencast im Repo, Diagramme aktuell, README final gelesen
