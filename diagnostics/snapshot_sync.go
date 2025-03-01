package diagnostics

import (
	"encoding/json"
	"net/http"
)

func SetupStagesAccess(metricsMux *http.ServeMux, diag *DiagnosticClient) {
	metricsMux.HandleFunc("/snapshot-sync", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		writeStages(w, diag)
	})
}

func writeStages(w http.ResponseWriter, diag *DiagnosticClient) {
	json.NewEncoder(w).Encode(diag.SnapshotDownload())
}
