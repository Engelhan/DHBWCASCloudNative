// Default fuer lokale Entwicklung (docker-compose, siehe README ## Lokal starten). Im Cluster
// ueberschreibt ein ConfigMap-Mount (charts/ticketflow-app/templates/frontend/configmap.yaml) diese
// Datei mit einem leeren String: das Ingress routet /events und /reservations bereits auf
// derselben Origin an catalog-api weiter (siehe templates/frontend/ingress.yaml), ein leerer
// API_BASE_URL macht fetch(`${API_BASE_URL}/events`) dann zu einem relativen same-origin-Request
// - kein CORS, keine Cluster-Hostnamen im Frontend-Code hartkodiert.
window.API_BASE_URL = "http://localhost:8080";
