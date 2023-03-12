package gwi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-billy/v5/osfs"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"

	"github.com/gorilla/mux"
)

// GitHandler is the interface with git that handles git operations
// like pull and push. To use this handler use the correct config options.
func (g *Gwi) infoRefsHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	user := mux.Vars(r)["user"]
	repo := mux.Vars(r)["repo"]
	service := mux.Vars(r)["service"]
	end, err := transport.NewEndpoint(user + "/" + repo)
	if err != nil {
		logger.Error("invalid URL", err.Error())
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}
	transport.UnsupportedCapabilities = []capability.Capability{
		capability.ThinPack,
	}

	gitServer := server.NewServer(server.NewFilesystemLoader(osfs.New(g.config.Root)))
	var sess transport.Session
	switch service {
	case "git-receive-pack":
		// needs auth
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
		if user != login {
			http.Error(w, "invalid repo", http.StatusUnauthorized)
			return
		}
		logger.Info("successful authentication")

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

		sess, err = gitServer.NewReceivePackSession(end, nil)
	case "git-upload-pack":
		sess, err = gitServer.NewUploadPackSession(end, nil)
	}
	if err != nil {
		logger.Error("session error:", err.Error())
		http.Error(w, "session error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	refs, err := sess.AdvertisedReferences()
	if err != nil {
		logger.Error("refs", err.Error())
		http.Error(w, "receive pack error", http.StatusBadRequest)
		return
	}
	refs.Prefix = [][]byte{
		[]byte("# service=" + service),
		pktline.Flush,
	}
	refs.Capabilities.Add(capability.NoDone)

	w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
	w.Header().Set("Accept-Encoding", "identity")
	if err := refs.Encode(w); err != nil {
		logger.Error("encode refs", err.Error())
		http.Error(w, "encode refs error", http.StatusInternalServerError)
		return
	}
	logger.Debug("sent", refs.References, *refs.Capabilities)
}

func (g *Gwi) receivePackHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	login, pass, ok := r.BasicAuth()
	user := mux.Vars(r)["user"]
	repo := mux.Vars(r)["repo"]
	if !ok || login == "" || pass == "" {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !g.vault.Validate(login, pass) {
		http.Error(w, "invalid login", http.StatusUnauthorized)
		return
	}
	if user != login {
		http.Error(w, "invalid repo", http.StatusUnauthorized)
		return
	}

	var body io.Reader
	var err error
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		body, err = gzip.NewReader(r.Body)
		if err != nil {
			w.Header().Add("Accept-encoding", "identity")
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}
	case "identity", "":
		body = r.Body
	default:
		w.Header().Add("Accept-encoding", "identity,gzip")
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")

	var stderr bytes.Buffer
	cmd := exec.Command("git", "receive-pack", "--stateless-rpc", repo)
	cmd.Dir = path.Join(g.config.Root, user)
	cmd.Stdout = w
	cmd.Stderr = &stderr
	cmd.Stdin = body
	if err = cmd.Run(); err != nil {
		logger.Error("HTTP.serviceRPC: fail to serve RPC", err.Error()+stderr.String())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (g *Gwi) uploadPackHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	user := mux.Vars(r)["user"]
	repo := mux.Vars(r)["repo"]

	var err error
	var body io.Reader
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		body, err = gzip.NewReader(r.Body)
		if err != nil {
			w.Header().Add("Accept-encoding", "identity")
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}
	case "identity", "":
		body = r.Body
	default:
		w.Header().Add("Accept-encoding", "identity,gzip")
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")

	var stderr bytes.Buffer
	cmd := exec.Command("git", "upload-pack", "--stateless-rpc", repo)
	cmd.Dir = path.Join(g.config.Root, user)
	cmd.Stdout = w
	cmd.Stderr = &stderr
	cmd.Stdin = body
	if err = cmd.Run(); err != nil {
		logger.Error("HTTP.serviceRPC: fail to serve RPC", err.Error()+stderr.String())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (g *Gwi) headHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	vars := mux.Vars(r)
	repoDir := path.Join(g.config.Root, vars["user"], vars["repo"])

	w.Header().Set("Content-Type", "text/plain")

	http.ServeFile(w, r, path.Join(repoDir, "HEAD"))
}

func (g *Gwi) fileHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	vars := mux.Vars(r)
	repoDir := path.Join(g.config.Root, vars["user"], vars["repo"])

	switch path.Ext(r.URL.Path) {
	case ".idx":
		w.Header().Set(
			"Content-Type",
			"application/x-git-packed-objects-toc",
		)
	case ".pack":
		w.Header().Set(
			"Content-Type",
			"application/x-git-packed-objects",
		)
	default:
		w.Header().Set("Content-Type", "text/plain")
	}

	http.ServeFile(w, r, path.Join(repoDir, vars["obj"]))
}

func (g *Gwi) objHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling object", r.Method, r.RequestURI)

	vars := mux.Vars(r)
	repoDir := path.Join(g.config.Root, vars["user"], vars["repo"])
	obj := vars["pre"] + "/" + vars["obj"]

	w.Header().Set("Content-Type", "application/x-git-loose-objects")

	http.ServeFile(w, r, path.Join(repoDir, "objects", obj))
}
