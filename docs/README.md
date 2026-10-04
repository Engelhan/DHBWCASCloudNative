# docs/

Ergänzende Diagramme und Nachweise, auf die die Root-`README.md` verweist. Das primäre
Architekturdiagramm ist [`architecture/architecture.drawio.svg`](architecture/architecture.drawio.svg),
eingebunden in der Root-README (Abschnitt `## Architektur`). Es ist ein SVG mit eingebettetem
draw.io-Modell: GitHub zeigt es als Bild, draw.io (Web/Desktop oder die VS-Code-Erweiterung
„Draw.io Integration") öffnet und speichert es direkt — keine separate Quelldatei, kein Export-Schritt.
Beim Speichern in draw.io das Format `.drawio.svg` beibehalten und „Diagramm einbetten" aktiviert lassen.

## [`architecture/`](architecture/)

- [`architecture.drawio.svg`](architecture/architecture.drawio.svg) — Gesamtarchitektur (siehe oben).
- [`pod-distribution.drawio.svg`](architecture/pod-distribution.drawio.svg) — Verteilung der Pods
  auf die Nodes/Regionen (P4), aus `kubectl get pods -A -o wide` abgeleitet. Ein Schnappschuss:
  nach Änderungen an Replicas oder Spread-Regeln neu erzeugen, sonst zeigt er nicht den Ist-Zustand
  — ein Diagramm, das den Ist-Zustand nicht trifft, zählt laut Bewertungsschema (Teil C) gegen uns.

Weitere Nachweise (z. B. der ArgoCD-App-Tree für W2) kommen ebenfalls hierher.
