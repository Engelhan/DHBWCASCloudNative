# docs/

Ergänzende Diagramme und Nachweise, auf die die Root-`README.md` verweist. Das primäre
Architekturdiagramm ist [`architecture/architecture.drawio.svg`](architecture/architecture.drawio.svg),
eingebunden in der Root-README (Abschnitt `## Architektur`). Es ist ein SVG mit eingebettetem
draw.io-Modell: GitHub zeigt es als Bild, draw.io (Web/Desktop oder die VS-Code-Erweiterung
„Draw.io Integration") öffnet und speichert es direkt — keine separate Quelldatei, kein Export-Schritt.
Beim Speichern in draw.io das Format `.drawio.svg` beibehalten und „Diagramm einbetten" aktiviert lassen.

## [`architecture/`](architecture/)

Das Architekturdiagramm (siehe oben) sowie Screenshots/Exports, die darüber hinausgehen (z.B. `kubectl get pods -o wide`
zur Knotenverteilung für P4, Grafana-Dashboard-Screenshot für P7, ArgoCD-App-Tree für W2). Werden
ergänzt, sobald der Cluster tatsächlich läuft — ein Diagramm, das den Ist-Zustand nicht trifft,
zählt laut Bewertungsschema (Teil C) gegen uns.
