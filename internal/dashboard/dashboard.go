package dashboard

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"phlo/internal/api"
	"phlo/internal/app"
	"phlo/internal/errs"
	"phlo/internal/site"
	"phlo/ui"
)

// Controller is the Phlo application surface the dashboard calls.
type Controller interface {
	Status() (api.Status, error)
	ListSites() ([]site.Site, error)
	CreateSite(account string) (string, error)
	AddSubdomain(account, name string) (string, error)
	SetHost(account, host string) (string, error)
	SetPHP(account, target, version string) (string, error)
	ApplySettings(patch app.SettingsPatch) (string, error)
	SetHomeDir(path string, copyFiles bool) (string, error)
	SetDataDir(database, path string, copyFiles bool) (string, error)
	ConfigFile(kind, version string) (api.ConfigFile, error)
	SaveConfigFile(kind, version, content string) (string, error)
	Start() (string, error)
	Stop() (string, error)
	Download(ctx context.Context, component, version string) (string, error)
	OpenTerminal(kind, account, target string) (string, error)
	Runtime() (api.Runtime, error)
	DockerStatus(ctx context.Context) (api.Docker, error)
	InstallVCRuntime(ctx context.Context) (string, error)
	OpenHeidiSQL() (string, error)
	OpenSQLyog() (string, error)
	OpenSQLyogInstaller() (string, error)
	SetNgrokToken(token string) (string, error)
	Share(host string) (string, error)
	StopShare() (string, error)
	RunCronTest() (string, error)
	Release(ctx context.Context) (api.Release, error)
	Update(ctx context.Context) (string, error)
}

type okResponse struct {
	OK      bool   `json:"ok"`
	Warning string `json:"warning,omitempty"`
}

// Handler serves the local dashboard and its API.
func Handler(controller Controller, onShutdown func(), onUpdated func()) http.Handler {
	mux := http.NewServeMux()
	apiHandler := &httpAPI{controller: controller, onShutdown: onShutdown, onUpdated: onUpdated}
	mux.HandleFunc("GET /api/health", apiHandler.health)
	mux.HandleFunc("GET /api/status", apiHandler.status)
	mux.HandleFunc("GET /api/sites", apiHandler.sites)
	mux.HandleFunc("POST /api/sites", apiHandler.createSite)
	mux.HandleFunc("POST /api/sites/{account}/subdomains", apiHandler.addSubdomain)
	mux.HandleFunc("PUT /api/sites/{account}/host", apiHandler.setHost)
	mux.HandleFunc("PUT /api/sites/{account}/php", apiHandler.setPHP)
	mux.HandleFunc("PUT /api/settings", apiHandler.setDefault)
	mux.HandleFunc("PUT /api/folders/home", apiHandler.setHomeDir)
	mux.HandleFunc("PUT /api/folders/{database}", apiHandler.setDataDir)
	mux.HandleFunc("GET /api/config-files/{kind}", apiHandler.configFile)
	mux.HandleFunc("PUT /api/config-files/{kind}", apiHandler.saveConfigFile)
	mux.HandleFunc("POST /api/services/start", apiHandler.start)
	mux.HandleFunc("POST /api/services/stop", apiHandler.stop)
	mux.HandleFunc("POST /api/shutdown", apiHandler.shutdown)
	mux.HandleFunc("GET /api/runtime", apiHandler.runtime)
	mux.HandleFunc("POST /api/runtime/download", apiHandler.download)
	mux.HandleFunc("GET /api/docker", apiHandler.docker)
	mux.HandleFunc("POST /api/vcruntime", apiHandler.vcRuntime)
	mux.HandleFunc("POST /api/terminal", apiHandler.terminal)
	mux.HandleFunc("POST /api/heidisql", apiHandler.heidisql)
	mux.HandleFunc("POST /api/sqlyog", apiHandler.sqlyog)
	mux.HandleFunc("POST /api/sqlyog/installer", apiHandler.sqlyogInstaller)
	mux.HandleFunc("PUT /api/ngrok/token", apiHandler.ngrokToken)
	mux.HandleFunc("POST /api/share", apiHandler.share)
	mux.HandleFunc("POST /api/share/stop", apiHandler.stopShare)
	mux.HandleFunc("POST /api/cronical/test", apiHandler.cronicalTest)
	mux.HandleFunc("GET /api/release", apiHandler.release)
	mux.HandleFunc("POST /api/update", apiHandler.update)
	mux.Handle("/", noCache(http.FileServer(http.FS(ui.Files))))
	return withSecurity(mux)
}

type httpAPI struct {
	controller Controller
	onShutdown func()
	onUpdated  func()
}

func (h *httpAPI) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *httpAPI) status(w http.ResponseWriter, _ *http.Request) {
	status, err := h.controller.Status()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *httpAPI) sites(w http.ResponseWriter, _ *http.Request) {
	sites, err := h.controller.ListSites()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sites)
}

func (h *httpAPI) createSite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account string `json:"account"`
	}
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.CreateSite(body.Account)
	writeMutation(w, warn, err)
}

func (h *httpAPI) addSubdomain(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFrom(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.AddSubdomain(account, body.Name)
	writeMutation(w, warn, err)
}

func (h *httpAPI) setHost(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFrom(w, r)
	if !ok {
		return
	}
	var body struct {
		Host *string `json:"host"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Host == nil {
		writeStatus(w, http.StatusBadRequest, "host is required")
		return
	}
	warn, err := h.controller.SetHost(account, *body.Host)
	writeMutation(w, warn, err)
}

func (h *httpAPI) setPHP(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFrom(w, r)
	if !ok {
		return
	}
	var body struct {
		Target  string  `json:"target"`
		Version *string `json:"version"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Target == "" || body.Version == nil {
		writeStatus(w, http.StatusBadRequest, "target and version are required")
		return
	}
	warn, err := h.controller.SetPHP(account, body.Target, *body.Version)
	writeMutation(w, warn, err)
}

func (h *httpAPI) setDefault(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DefaultPHP       *string           `json:"defaultPhp"`
		Nginx            *string           `json:"nginx"`
		MariaDB          *string           `json:"mariadb"`
		Redis            *string           `json:"redis"`
		Memcached        *string           `json:"memcached"`
		Mailpit          *string           `json:"mailpit"`
		HTTPPort         *int              `json:"httpPort"`
		SSLPort          *int              `json:"sslPort"`
		HostIP           *string           `json:"hostIp"`
		MySQLPort        *int              `json:"mysqlPort"`
		RedisPort        *int              `json:"redisPort"`
		MemcachedPort    *int              `json:"memcachedPort"`
		MailpitSMTPPort  *int              `json:"mailpitSmtpPort"`
		MailpitUIPort    *int              `json:"mailpitUiPort"`
		MongoDBPort      *int              `json:"mongodbPort"`
		PostgresPort     *int              `json:"postgresPort"`
		SSLEnabled       *bool             `json:"sslEnabled"`
		MariaDBEnabled   *bool             `json:"mariadbEnabled"`
		RedisEnabled     *bool             `json:"redisEnabled"`
		MemcachedEnabled *bool             `json:"memcachedEnabled"`
		MailpitEnabled   *bool             `json:"mailpitEnabled"`
		MongoDBEnabled   *bool             `json:"mongodbEnabled"`
		PostgresEnabled  *bool             `json:"postgresEnabled"`
		CronEnabled      *bool             `json:"cronEnabled"`
		Tools            map[string]string `json:"tools"`
		ToolPath         map[string]bool   `json:"toolPath"`
	}
	if !decode(w, r, &body) {
		return
	}
	patch := app.SettingsPatch{
		DefaultPHP:       body.DefaultPHP,
		Nginx:            body.Nginx,
		MariaDB:          body.MariaDB,
		Redis:            body.Redis,
		Memcached:        body.Memcached,
		Mailpit:          body.Mailpit,
		HTTPPort:         body.HTTPPort,
		SSLPort:          body.SSLPort,
		HostIP:           body.HostIP,
		MySQLPort:        body.MySQLPort,
		RedisPort:        body.RedisPort,
		MemcachedPort:    body.MemcachedPort,
		MailpitSMTPPort:  body.MailpitSMTPPort,
		MailpitUIPort:    body.MailpitUIPort,
		SSLEnabled:       body.SSLEnabled,
		MariaDBEnabled:   body.MariaDBEnabled,
		RedisEnabled:     body.RedisEnabled,
		MemcachedEnabled: body.MemcachedEnabled,
		MailpitEnabled:   body.MailpitEnabled,
		MongoDBPort:      body.MongoDBPort,
		PostgresPort:     body.PostgresPort,
		MongoDBEnabled:   body.MongoDBEnabled,
		PostgresEnabled:  body.PostgresEnabled,
		CronEnabled:      body.CronEnabled,
		Tools:            body.Tools,
		ToolPath:         body.ToolPath,
	}
	if patch.DefaultPHP == nil && patch.Nginx == nil && patch.MariaDB == nil && patch.Redis == nil && patch.Memcached == nil && patch.Mailpit == nil &&
		patch.HTTPPort == nil && patch.SSLPort == nil && patch.HostIP == nil && patch.MySQLPort == nil && patch.RedisPort == nil && patch.MemcachedPort == nil &&
		patch.MailpitSMTPPort == nil && patch.MailpitUIPort == nil && patch.SSLEnabled == nil &&
		patch.MariaDBEnabled == nil && patch.RedisEnabled == nil && patch.MemcachedEnabled == nil && patch.MailpitEnabled == nil &&
		patch.MongoDBPort == nil && patch.PostgresPort == nil && patch.MongoDBEnabled == nil && patch.PostgresEnabled == nil &&
		patch.CronEnabled == nil &&
		len(patch.Tools) == 0 && len(patch.ToolPath) == 0 {
		writeStatus(w, http.StatusBadRequest, "a settings field is required")
		return
	}
	warn, err := h.controller.ApplySettings(patch)
	writeMutation(w, warn, err)
}

func (h *httpAPI) start(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.Start()
	writeMutation(w, warn, err)
}

func (h *httpAPI) stop(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.Stop()
	writeMutation(w, warn, err)
}

func (h *httpAPI) shutdown(w http.ResponseWriter, _ *http.Request) {
	if h.onShutdown == nil {
		writeStatus(w, http.StatusNotFound, "shutdown is not available")
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go h.onShutdown()
}

func (h *httpAPI) runtime(w http.ResponseWriter, _ *http.Request) {
	view, err := h.controller.Runtime()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *httpAPI) docker(w http.ResponseWriter, r *http.Request) {
	status, err := h.controller.DockerStatus(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *httpAPI) vcRuntime(w http.ResponseWriter, r *http.Request) {
	warn, err := h.controller.InstallVCRuntime(r.Context())
	writeMutation(w, warn, err)
}

func (h *httpAPI) terminal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind    string `json:"kind"`
		Account string `json:"account"`
		Target  string `json:"target"`
	}
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.OpenTerminal(body.Kind, body.Account, body.Target)
	writeMutation(w, warn, err)
}

func (h *httpAPI) heidisql(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.OpenHeidiSQL()
	writeMutation(w, warn, err)
}

func (h *httpAPI) sqlyog(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.OpenSQLyog()
	writeMutation(w, warn, err)
}

func (h *httpAPI) sqlyogInstaller(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.OpenSQLyogInstaller()
	writeMutation(w, warn, err)
}

func (h *httpAPI) ngrokToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.SetNgrokToken(body.Token)
	writeMutation(w, warn, err)
}

func (h *httpAPI) share(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host string `json:"host"`
	}
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.Share(body.Host)
	writeMutation(w, warn, err)
}

func (h *httpAPI) stopShare(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.StopShare()
	writeMutation(w, warn, err)
}

func (h *httpAPI) cronicalTest(w http.ResponseWriter, _ *http.Request) {
	warn, err := h.controller.RunCronTest()
	writeMutation(w, warn, err)
}

func (h *httpAPI) release(w http.ResponseWriter, r *http.Request) {
	info, err := h.controller.Release(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *httpAPI) update(w http.ResponseWriter, r *http.Request) {
	msg, err := h.controller.Update(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true, Warning: msg})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	if h.onUpdated != nil {
		go h.onUpdated()
	}
}

func (h *httpAPI) download(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Component string `json:"component"`
		Version   string `json:"version"`
	}
	if !decode(w, r, &body) {
		return
	}
	warn, err := h.controller.Download(r.Context(), body.Component, body.Version)
	writeMutation(w, warn, err)
}

func accountFrom(w http.ResponseWriter, r *http.Request) (string, bool) {
	account := r.PathValue("account")
	if err := site.ValidateAccount(account); err != nil {
		writeError(w, err)
		return "", false
	}
	return account, true
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		writeStatus(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func writeMutation(w http.ResponseWriter, warning string, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true, Warning: warning})
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if _, ok := errs.As(err); ok {
		status = http.StatusBadRequest
	}
	writeStatus(w, status, err.Error())
}

func writeStatus(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("phlo: write json: %v", err)
	}
}

func withSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !csrfOK(r) {
			writeStatus(w, http.StatusForbidden, "missing X-Phlo header")
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("phlo: panic: %v", rec)
				writeStatus(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func csrfOK(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return r.Header.Get("X-Phlo") == "1"
	}
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
