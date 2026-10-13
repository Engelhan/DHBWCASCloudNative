# docs/

Ergänzende Diagramme und Nachweise, auf die die Root-`README.md` verweist. Das primäre
Architekturdiagramm ist [`architecture/architecture.drawio.svg`](architecture/architecture.drawio.svg),
eingebunden in der Root-README (Abschnitt `## Architektur`). Es ist ein SVG mit eingebettetem
draw.io-Modell: GitHub zeigt es als Bild, draw.io (Web/Desktop oder die VS-Code-Erweiterung
„Draw.io Integration") öffnet und speichert es direkt — keine separate Quelldatei, kein Export-Schritt.
Beim Speichern in draw.io das Format `.drawio.svg` beibehalten und „Diagramm einbetten" aktiviert lassen.

## [`architecture/`](architecture/)

- [`architecture.drawio.svg`](architecture/architecture.drawio.svg) — Gesamtarchitektur (siehe oben).
- [`pod-distribution.drawio.svg`](architecture/pod-distribution.drawio.svg) — Beispiel für die
  Verteilung der Pods auf die Nodes/Regionen (P4): ein realer Abzug aus `kubectl get pods -A -o wide`,
  aber nur eine Momentaufnahme — die konkrete Zuordnung ändert sich bei jedem Rescheduling, gleich
  bleiben die Regeln. Nach Änderungen an Replicas oder Spread-Regeln neu erzeugen, damit das
  Beispiel zu den Regeln passt.
