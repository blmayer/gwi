package gwi

import (
	"net/http"
	"net/http/cgi"
	"os"

	"blmayer.dev/git/gwi/internal/logger"
)

func (g *Gwi) Private(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		login, pass, ok := r.BasicAuth()
		if !ok || login == "" || pass == "" {
			w.Header().Set("WWW-Authenticate", "Basic")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if !g.vault.Validate(login, pass) {
			http.Error(w, "invalid login", http.StatusUnauthorized)
			return
		}

		logger.Info("successful authentication")
		h(w, r)
	}
}

func (g *Gwi) GitCGIHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("CGI handling", r.RequestURI)

	env := []string{
		"GIT_PROJECT_ROOT=" + g.config.Root,
		"GIT_HTTP_EXPORT_ALL=1",
		"REMOTE_USER=" + g.config.Domain,
	}

	logger.Debug("using root: ", g.config.CGIPrefix)
	logger.Debug("cgiPath: ", g.config.CGIRoot)
	logger.Debug("using env: ", env)
	handler := &cgi.Handler{
		Path:   g.config.CGIRoot,
		Root:   g.config.CGIPrefix,
		Env:    env,
		Stderr: os.Stderr,
	}

	handler.ServeHTTP(w, r)
}
