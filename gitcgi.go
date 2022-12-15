package gwi

import (
	"net/http"
	"net/http/cgi"
	"os"
	"path"
	"strings"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/gorilla/mux"
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

	login, pass, ok := r.BasicAuth()
	user := mux.Vars(r)["user"]
	repo := mux.Vars(r)["repo"]
	if strings.Contains(r.RequestURI, "git-receive-pack") {
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

		if user != login {
			http.Error(w, "invalid repo", http.StatusUnauthorized)
			return
		}
	}

	// create repo if it doesn't exists
	repoDir := path.Join(g.config.Root, user, repo)
	if _, err := os.Stat(repoDir); err != nil {
		logger.Info("repo stat", err.Error(), "initializing repo")

		os.Mkdir(repoDir, os.ModeDir|0o700)
		r, err := git.PlainInit(repoDir, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main"))
		if err := r.Storer.SetReference(h); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg, err := r.Config()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg.Init.DefaultBranch = "main"
		r.Storer.SetConfig(cfg)
	}

	env := []string{
		"GIT_PROJECT_ROOT=" + g.config.Root,
		"GIT_HTTP_EXPORT_ALL=1",
		"REMOTE_USER=" + login,
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
