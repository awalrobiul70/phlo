package dashboard

import "net/http"

// configFile returns one custom settings file: php (with ?version=), mariadb, postgres, or mongodb.
func (h *httpAPI) configFile(w http.ResponseWriter, r *http.Request) {
	file, err := h.controller.ConfigFile(r.PathValue("kind"), r.URL.Query().Get("version"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func (h *httpAPI) saveConfigFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string  `json:"version"`
		Content *string `json:"content"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Content == nil {
		writeStatus(w, http.StatusBadRequest, "content is required")
		return
	}
	message, err := h.controller.SaveConfigFile(r.PathValue("kind"), body.Version, *body.Content)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": message})
}
