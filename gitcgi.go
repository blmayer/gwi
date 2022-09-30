package gwi

import (
	"net/http"
	"net/http/cgi"
	"os"

	"blmayer.dev/gwi/internal/logger"
)

var (
	gitUser    string
	backendCGI = "/usr/lib/git-core/git-http-backend"

	gitPass = os.Getenv("GIT_PASS")
)

func private(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != gitUser || pass != gitPass {
			w.Header().Set("WWW-Authenticate", "Basic")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		logger.Info("successful authentication")
		h(w, r)
	}
}

func (g *Gwi) GitCGIHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("CGI handling", r.RequestURI)

	env := []string{
		"GIT_PROJECT_ROOT=" + g.gitRoot,
		"GIT_HTTP_EXPORT_ALL=1",
		"REMOTE_USER=blmayer",
	}

	handler := &cgi.Handler{
		Path:   backendCGI,
		Root:   "/",
		Env:    env,
		Stderr: os.Stderr,
	}

	handler.ServeHTTP(w, r)
}