package gwi

import (
	"io"
	"net/http"
	"compress/gzip"
	"os"
	"path"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-billy/v5/osfs"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
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

	gitServer := server.NewServer(server.NewFilesystemLoader(osfs.New(g.config.Root)))
	var sess transport.Session
	switch service {
	case "git-receive-pack":
		sess, err = gitServer.NewReceivePackSession(end, nil)
	case "git-upload-pack":
		sess, err = gitServer.NewUploadPackSession(end, nil)
	}
	if err != nil {
		logger.Error("session", err.Error())
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}

	refs, err := sess.AdvertisedReferences()
	if err != nil {
		logger.Error("refs", err.Error())
		http.Error(w, "receive pack error", http.StatusBadRequest)
		return
	}
	refs.Prefix = [][]byte{
		[]byte("# service="+service),
		pktline.Flush,
	}
	refs.Capabilities.Add("no-thin")
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
	logger.Info("successful authentication")

	if user != login {
		http.Error(w, "invalid repo", http.StatusUnauthorized)
		return
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

	gitServer := server.NewServer(server.NewFilesystemLoader(osfs.New(g.config.Root)))
	end, err := transport.NewEndpoint(user + "/" + repo)
	if err != nil {
		logger.Error("invalid URL", err.Error())
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}
	sess, err := gitServer.NewReceivePackSession(end, nil)
	if err != nil {
		logger.Error("session", err.Error())
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}

	upr := packp.NewReferenceUpdateRequest()
	if err := upr.Decode(r.Body); err != nil {
		logger.Error("reference decode", err.Error())
		http.Error(w, "reference decode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Debug("request:", upr.Commands, *upr.Capabilities)

	res, err := sess.ReceivePack(r.Context(), upr)
	if err != nil {
		logger.Error("receive pack", err.Error())
		http.Error(w, "receive pack: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	if err := res.Encode(w); err != nil {
		logger.Error("encode response", err.Error())
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
	logger.Debug("sent", *res, res.CommandStatuses)
}

func (g *Gwi) uploadPackHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	user := mux.Vars(r)["user"]
	repo := mux.Vars(r)["repo"]

	gitServer := server.NewServer(server.NewFilesystemLoader(osfs.New(g.config.Root)))
	end, err := transport.NewEndpoint(user + "/" + repo)
	if err != nil {
		logger.Error("invalid URL", err.Error())
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}
	sess, err := gitServer.NewUploadPackSession(end, nil)
	if err != nil {
		logger.Error("session", err.Error())
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}

	var body io.Reader
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		body, err = gzip.NewReader(r.Body)
		if err != nil {
			logger.Error(err) 
			w.Header().Add("Accept-encoding", "identity")
			http.Error(w, "", http.StatusUnsupportedMediaType)
			return
		}
	case "identity", "":
		body = r.Body
	}

	upr := packp.NewUploadPackRequest()
	if err := upr.Decode(body); err != nil {
		logger.Error("upload decode", err.Error())
		http.Error(w, "upload decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	logger.Debug("request:", upr.Wants, *upr.Capabilities)

	res, err := sess.UploadPack(r.Context(), upr)
	if err != nil {
		logger.Error("upload pack", err.Error())
		http.Error(w, "upload pack: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	if err := res.Encode(w); err != nil {
		logger.Error("encode response", err.Error())
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
	logger.Debug("sent", res.ServerResponse, res.ACKs)
}

func (g *Gwi) headHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	vars := mux.Vars(r)
	repoDir := path.Join(g.config.Root, vars["user"], vars["repo"])

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	head, err := repo.Head()
	if err != nil {
		logger.Error("git head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(head.Hash().String()))
}

func (g *Gwi) infoHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("git handling", r.Method, r.RequestURI)

	vars := mux.Vars(r)
	objPath := path.Join(
		g.config.Root, 
		vars["user"],
		vars["repo"], 
		"objects", 
		vars["obj"],
	)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeFile(w, r, objPath)
}
