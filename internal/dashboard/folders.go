package dashboard

import "net/http"

type folderBody struct {
	Path string `json:"path"`
	Copy *bool  `json:"copy"`
}

func (h *httpAPI) setHomeDir(w http.ResponseWriter, r *http.Request) {
	var body folderBody
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.SetHomeDir(body.Path, body.Copy == nil || *body.Copy)
	writeMutation(w, warn, err)
}

// setDataDir moves the data folder of mariadb, postgres, or mongodb.
func (h *httpAPI) setDataDir(w http.ResponseWriter, r *http.Request) {
	var body folderBody
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.SetDataDir(r.PathValue("database"), body.Path, body.Copy == nil || *body.Copy)
	writeMutation(w, warn, err)
}
