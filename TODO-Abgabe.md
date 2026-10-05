# Offene Punkte bis zur Abgabe

Abgleich des Repos mit der Aufgabenstellung V2 (`2026_07_23_Aufgabenstellung - v2.pdf`),
Stand 04.10.2026. Sortiert nach Auswirkung auf die Bewertung. Datei vor der Abgabe löschen.

Legende: 🔴 blockiert Punkte oder macht Teile unbewertbar · 🟠 kostet wahrscheinlich Punkte ·
🟡 Feinschliff

## Überblick

| Bereich | Punkte | Stand |
|---|---|---|
| Teil A – P1 Anwendung/Domäne | 8 | erfüllt |
| Teil A – P2 IaC, frisches Projekt | 15 | umgesetzt, **Test auf frischem Projekt fehlt** |
| Teil A – P3 Requests/Limits, Tags, Config | 10 | erfüllt |
| Teil A – P4 Platzierung + Nachweis | 10 | umgesetzt, Diagramm aktuell |
| Teil A – P5 Resilienz | 12 | umgesetzt, **Nachweis fehlt in README/Screencast** |
| Teil B – Wahlthema | 10 | W2 (GitOps) gewählt, **Nachweis im Screencast fehlt** |
| Teil C – Doku/Darstellung | 35 | **Screencast fehlt** (5 P.) |
| Bonus | ≤ 10 | B2/B3 (W7)/B4 beansprucht; **B2 verlangt Grafana-Dashboard im Screencast** |

---

## 🔴 Kritisch

### 1. Screencast fehlt komplett (Teil C, 5 Punkte + Nachweise für P5, Teil B, Bonus)

`screencast/` enthält nur die README. Vorgaben (Abschnitt 6.3 / Teil C):

- [ ] max. **7 Minuten**, deutsch oder englisch, mit Ton
- [ ] **Kapitelmarken** (werden in Teil C explizit bewertet)
- [ ] **jedes Gruppenmitglied spricht** (explizit bewertet)
- [ ] nur **Live-Terminal**, keine Standbilder/Folien
- [ ] **Pflichtszene:** Pod löschen → Anwendung erholt sich → Daten sind noch da
      (z. B. Reservierung anlegen, `reservation-worker`- und Postgres-Pod löschen, Reservierung
      ist danach noch abrufbar)
- [ ] Szene für das Wahlthema W2 (Teil B: „Wirkung im Betrieb nachgewiesen“, 4 Punkte):
      `kubectl scale`/`kubectl edit` am Cluster vorbei → ArgoCD dreht es zurück
- [ ] Szene für B3 (W7): NetworkPolicy-Negativprobe und/oder OpenBao-Failover
- [ ] Szene für B2: Last erzeugen → Replicas wachsen (siehe Punkt 4)
- [ ] mit ffmpeg/HandBrake komprimieren, **im Repo ablegen** (kein externer Video-Link erlaubt),
      Gesamt-Repo < 100 MB

### 2. ~~Wahlthema festlegen~~ ✅ erledigt (05.10.)

Entscheidung: **W2 (GitOps)** ist das gewertete Wahlthema in `## Entscheidungen`, **W7
(Cluster-Härtung)** wird zusätzlich als **B3 „Weiteres Wahlthema“** im Bonus beansprucht. W1
(Packaging) entfällt als Thema, die Begründung steht jetzt als „P3 – Ergänzung“; die `TODO`-Kästen in `## Entscheidungen` und `## Bonus` sind
entfernt, W2 hat einen „*Nachweis im Betrieb*“-Satz.

### 3. ~~README-Struktur: `## ToDo` muss raus~~ ✅ erledigt (04.10.)

Der Abschnitt ist entfernt, die offenen Punkte daraus stehen unten unter „Aus dem Team-ToDo der
README übernommen“.

- [ ] Status-Kasten über `## Problem` vor der Abgabe prüfen bzw. entfernen.

### 4. B2 Autoscaling: Grafana-Dashboard wird im Screencast verlangt

Abschnitt 6.3: „Wenn Sie den Bonus B1 oder B2 beanspruchen: Last erzeugen, Replicas wachsen,
**Wirkung im Grafana-Dashboard zeigen**“. Der Observability-Stack wurde am 29.09. entfernt; die
README zeigt die Wirkung stattdessen über `kubectl`/AKHQ. Das widerspricht der Vorgabe.

- [ ] Entscheiden: (a) minimalen Prometheus + Grafana wieder aufbauen (Dashboard mit
      Consumer-Lag und Replica-Zahl reicht; die `ServiceMonitor`/`PodMonitor`-Schalter in den Charts
      existieren noch, ebenso `np.ingress.prometheus` in `charts/network-policies`), oder
      (b) B2 bewusst nur mit `kubectl` zeigen und das Risiko geringerer Bonuspunkte tragen.
- [ ] Bei (a) prüfen, ob sich damit auch B1 (Observability, bis 6 P.) lohnt: zusätzlich
      Dashboard als Code und **eine Alert-Regel, die auch feuert**.

### 5. P2: kompletter Aufbau auf einem frischen Projekt testen (15 Punkte, sonst A+B unbewertbar)

„Wenn der Stack sich nicht aus dem Repository heraus aufbauen lässt, sind Teil A und Teil B nicht
bewertbar. Probieren Sie das rechtzeitig auf einem frisch erzeugten Cluster.“ Seit dem letzten
Neuaufbau kam viel Neues dazu, das nur auf dem gewachsenen Cluster lief: gestufter Rollout
(`gitops_rollout`), Control-Plane-Taint, ≥ 2 Replicas mit harter Verteilung (Ressourcenbedarf),
zwei ApplicationSet-Dateien, AKHQ-JWT-Secret aus OpenBao.

- [ ] Neues OpenStack-Projekt bzw. alle VMs löschen, nur mit den README-Schritten aus
      `## Betrieb` aufbauen, jede Abweichung in README/Code korrigieren.
- [ ] Prüfen, ob alles auf 3 Worker passt (Memory-Requests wuchsen durch die zweiten Replicas um
      ~2,2 Gi).
- [ ] Prüfen, ob `gitops_rollout` innerhalb von 30 min je Stufe grün wird.

---

## 🟠 Wichtig

### 6. Nachweise im Betrieb in `## Entscheidungen` ergänzen

Teil A verlangt je Pflichtthema „im Betrieb nachgewiesen“. P4, B2, B3 und B4 haben einen
„*Nachweis im Betrieb*“-Satz, die anderen nicht.

- [ ] **P5:** konkrete Kommandos für Pod-Löschung und Rolling Update ohne Ausfall (z. B.
      `kubectl rollout restart` während eines laufenden Lasttests, k6 zeigt keine Fehler; CNPG-
      Primary löschen → Failover, Daten noch da).
- [ ] **P1/P3:** kurzer Nachweis (Images mit SHA-Tag in `kubectl get pods -o jsonpath=…image`,
      ConfigMap/Secret-Referenzen).
- [ ] **W2:** Nachweis-Kommandos stehen in der README, einmal live ausprobieren
      (Drift-Rückdrehen dauert je nach ArgoCD-Reconcile-Intervall).

### 7. ~~Diagramme an den aktuellen Stand anpassen~~ ✅ erledigt (04.10.)

Beide Diagramme in `docs/architecture/` zeigen den Stand nach Taint und Mindest-Replicas. Nach
jeder Änderung an Replicas, Spread-Regeln oder ApplicationSets erneut prüfen.

### 8. `## Grenzen`: Verhalten unter echter Last konkret benennen

Vorgabe: „Was die Lösung nicht kann und **was Sie unter echter Last erwarten**“. In der README
steht noch `TODO: nach Lasttests unter echten Bedingungen konkretisieren`.

- [ ] Lasttest auf dem Cluster fahren (`src/load-generator/run.sh`), beobachtete Werte notieren
      (Reservierungen/s, Lag-Verlauf, Zeit bis Skalierung, Engpass Postgres-Primary) und als
      konkrete Grenze formulieren.
- [ ] Optional daraus **B6 „Belastbare Messung“** machen (Lastreihe, Grafik, Interpretation;
      bis 5 P.).

### 9. Fremden Code in `## Entscheidungen` benennen (Abschnitt 9, Formales)

„Fremder Code ist erlaubt, wenn er in `## Entscheidungen` als solcher benannt wird.“ Aktuell
nicht benannt:

- [ ] Addon-Konfiguration portiert aus `pfisterer/k3s-dhbw-cloud-role` (Longhorn, cert-manager,
      external-dns; siehe `gitops/README.md`)
- [ ] JMX-Exporter-Regeln (`charts/kafka-cluster/templates/messaging/kafka-metrics-configmap.yaml`)
      aus den Strimzi-Beispielen bzw. einer Referenzkonfiguration
- [ ] AKHQ-RBAC-Rollen aus AKHQs `application.example.yml`
- [ ] ggf. weitere übernommene Snippets (Team fragen)

### 10. Lokale Commits mit dem Remote zusammenführen und pushen

Lokal liegen ungepushte Commits (README-Bereinigung, ApplicationSet-Aufteilung, englische
Kommentare usw.). Der Cluster läuft bereits mit Image-Tag `6a24f197`, lokal steht in
`gitops/env/cluster.yaml` noch `26fced0f` — auf dem Remote gibt es also mindestens einen neueren
CI-Commit (Tag-Bump), der lokal fehlt.

- [ ] `git pull --rebase` (Konflikt nur in der `imageTag`-Zeile möglich), dann pushen.
- [ ] Danach prüfen, dass alle Applications wieder Synced/Healthy sind.

### 11. Zugang für den Prüfer

Abgabe als GitHub-Link verlangt „öffentlich oder Zugriff für Github-User `pfisterer`“; das Repo
ist privat.

- [ ] `pfisterer` als Collaborator (Lesezugriff) hinzufügen oder ZIP abgeben.
- [ ] Bei ZIP: max. 100 MB; lokale Artefakte wie `infra/terraform/terraform.exe` (115 MB) und
      `.terraform/` sind gitignored, aber nicht ins ZIP packen (z. B. `git archive` verwenden).

---

## 🟡 Feinschliff

### 12. Rechtschreibung und Konsistenz (Teil C, 2 Punkte)

- [ ] README einmal komplett Korrektur lesen (Mischung aus Umlauten und `ae/oe/ue` in
      Code-Kommentaren, sehr lange Sätze in P4/W2/B3).
- [ ] Bonus-Nummerierung prüfen: Die README nutzt B2 (Autoscaling), B3 (W7) und B4 (Kafka/Strimzi)
      passend zur V2-Tabelle.

### 13. Nachvollziehbarkeit für Fremde (Teil C, 4 Punkte)

- [ ] `## Betrieb` einmal von jemandem durchgehen lassen, der das Projekt nicht kennt.
- [ ] Manuelle Vorbedingungen ehrlich kennzeichnen: DNS-Zone im Self-Service-Portal registrieren,
      ghcr.io-Packages auf „Public“ stellen, Deploy-Key bei GitHub hinterlegen (P2 verbietet
      Portal-Klicks für VMs/Netz/Cluster; diese Schritte sind außerhalb davon, sollten aber
      begründet sein).

### 14. Commit-Verlauf / Beiträge

„Alle Nutzer der Gruppe sollen dort mit ihren Beiträgen erkennbar sein.“ Aktuelle Verteilung:
Xaron 188, Carlos-Ku9 71, Philipp Stiegler 31, alea563-blipp 3 Commits.

- [ ] Sicherstellen, dass jedes Mitglied unter einem erkennbaren Namen committet (ggf.
      `.mailmap` für Pseudonyme wie `Xaron` → Klarname).

### 15. Offene technische Punkte aus der Doku

- [ ] Dediziertes ArgoCD-`AppProject` statt `project: default` (in `gitops/README.md` als TODO).
- [ ] OpenBao-HA-Failover zwischen entsiegelten Standbys live testen (in CLAUDE.md als nicht
      verifiziert vermerkt; wird in B3 als Nachweis behauptet).
- [ ] B4-Negativprobe (falscher `KafkaUser` schreibt auf fremdes Topic → Autorisierungsfehler)
      einmal tatsächlich ausführen, bevor sie im Screencast gezeigt wird.

---

## Aus dem Team-ToDo der README übernommen

Der frühere Abschnitt `## ToDo` der Root-README (Stand 23.06.) – hier nur die Punkte, die noch
offen sind. Erledigt und deshalb nicht übernommen: Kafka als neue Technologie dokumentiert (jetzt
B4), B2 Autoscaling dokumentiert, alle Aufgaben aus der Verteilung (Frontend, API + Producer,
Consumer mit DB-Schema, Message-Service, Kafka-Cluster mit Schema-Registry und AKHQ,
Postgres-Cluster), Autoscaler (KEDA), erster Wurf des Sturm-Teils (`Laborbericht_teil_sturm.md`,
deckt alle drei Fragen ab).

**Reviews durch die Verantwortlichen** – technisch umgesetzt, aber laut Team-ToDo noch zu prüfen:

- [ ] P3 – Niklas prüft (Requests/Limits, SHA-Tags, ConfigMap/Secret)
- [ ] P4 – jeder für sein Helm-Chart (harte Regions-Verteilung, ≥ 2 Replicas, hergeleitete
      Requests/Limits)
- [ ] P5 – jeder für sein Helm-Chart (Probes, PVC/StatefulSet, PDB, Pod-Verlust ohne Datenverlust)
- [ ] W2 – Niklas prüft, ob GitOps so passt

**Entscheidungen:**

- [x] „B4 → Entscheidung welches Wahlthema wir nehmen W1 oder W7“ – W2 als Wahlthema, W7 als B3, siehe Punkt 2

**Screencast:**

- [ ] „Alle zusammen“ – siehe Punkt 1 (jedes Mitglied muss sprechen)

**Bonus-Ideen aus dem Team-ToDo, noch nicht umgesetzt:**

- [ ] Observability mit OpenTelemetry (Logging, Metriken, evtl. Traces) – der Stack wurde am
      29.09. entfernt; Entscheidung hängt an Punkt 4 (B1 bzw. B4-Richtung „Tracing“)
- [ ] Chaos-Experiment (B7, bis 5 P.): Node oder Dienst gezielt zerstören, Erwartung vorher
      formulieren, Abweichung diskutieren – passt zur Pflichtszene „Pod löschen“
- [ ] Helm-Charts packagen/veröffentlichen (B8 „Wiederverwendbarer Baustein“, bis 5 P.):
      verlangt ein veröffentlichtes, für Dritte dokumentiertes Chart – bisher nur intern genutzt.
      W1 (Packaging) wird nicht beansprucht, eine Überschneidung mit B8 gibt es also nicht.

## Außerhalb dieser Aufgabenstellung

- `Laborbericht_teil_sturm.md` (Architekturreflexion, Datensicherheit/DSGVO, Cloud-Native-Pattern)
  ist ein separater Prüfungsteil und nicht Gegenstand dieser PDF; Stand dort separat prüfen.

## Bereits erfüllt (zur Orientierung)

- Zwei+ eigene Services (catalog-api, reservation-worker, notification-worker, frontend), über
  Kafka/HTTP verbunden; zustandsbehaftete Dienste Postgres (CNPG) und Kafka (Strimzi)
- Künstlicher Lastpfad (`src/load-generator`), fachlicher Zweck in einem Satz (`## Problem`,
  9 Zeilen ≤ 10)
- Images bauen per GitHub Actions aus `src/`, Tag = Git-SHA, keine `:latest`-Referenzen in Charts
- Terraform/OpenTofu + Ansible, ein Ablauf (`terraform apply` + `ansible-playbook site.yml`)
- Requests/Limits mit hergeleiteten Zahlen; Konfiguration über ConfigMap/Secret
- Harte, domänenbegründete Platzierung (Kafka/OpenBao/Postgres je Region, alle Dienste ≥ 2
  Replicas hart verteilt), mehrere Worker, Control-Plane getaintet
- Probes, StatefulSets/PVCs, PDBs, idempotente Verarbeitung, RF 3 / min.insync 2
- Keine externen Cloud-Dienste (nur ghcr.io als Registry und GitHub als Git-Hoster)
- Live-Zustand am 04.10.: alle 21 Applications Synced/Healthy, App-Dienste mit je 2 Replicas
