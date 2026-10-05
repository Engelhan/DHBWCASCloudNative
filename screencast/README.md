# screencast/

Hier liegt am Ende die Screencast-Datei (z.B. `demo.mp4`), maximal 7 Minuten, mit Ton, deutsch
oder englisch. Pflichtszenen laut Aufgabenstellung:

1. Pod löschen → Anwendung erholt sich → Daten sind noch da (P5-Nachweis).
2. Last erzeugen (`src/load-generator/`) → Replicas wachsen (Bonus B2 Autoscaling, KEDA) —
   gezeigt über `kubectl get pods -w`/`scaledobject` und den Consumer-Lag in AKHQ, kein
   Grafana-Dashboard (siehe Root-`README.md` ## Grenzen).

Zusätzlich je eine kurze Nachweis-Szene für W2 (Drift wird zurückgedreht), B4 (Kafka-ACL-Negativprobe
mit Strimzi-`KafkaUser`) und B3/W7 (NetworkPolicy, OpenBao). Kapitel, Sprecher, Kommandos und
Sprechtexte: [`SCRIPT.md`](SCRIPT.md).

Alle Szenen live im Terminal, keine vorproduzierten Standbilder oder Folien.

## Komprimierung vor dem Commit

Das Repository ist auf 100MB begrenzt. Vor dem Ablegen hier mit `ffmpeg` komprimieren, z.B.:

```bash
ffmpeg -i raw-recording.mov -vcodec libx264 -crf 28 -preset slower -acodec aac -b:a 128k demo.mp4
```

`crf`-Wert ggf. anpassen (höher = kleinere Datei, niedrigere Qualität) und die resultierende
Dateigröße gegen das 100MB-Limit für das **gesamte** Repository prüfen, nicht nur die Videodatei
allein.

`TODO`: Datei ergänzen, sobald Infrastruktur und Anwendung stehen.
