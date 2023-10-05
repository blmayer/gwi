package gwi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-billy/v5/osfs"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
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

	if r.Header.Get("Git-Protocol") == "version=2" {
		caps, err := sess.AdvertisedCapabilities()
		if err != nil {
			logger.Error("caps error:", err.Error())
			http.Error(w, "caps error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		caps.Capabilities.Add(capability.Agent, "git/gwi")
		caps.Capabilities.Add(capability.Fetch, capability.Shallow.String(), capability.Filter.String(), capability.WaitForDone.String())
		caps.Capabilities.Add(capability.LsRefs, "unborn")
		caps.Capabilities.Add(capability.ObjectFormat, "sha1")
		caps.Capabilities.Add(capability.ServerOption)

		caps.Service = service
		w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
		if err := caps.Encode(w); err != nil {
			logger.Error("encode caps", err.Error())
			http.Error(w, "encode caps error", http.StatusInternalServerError)
		}
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
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")

	if err := upr.Decode(body); err != nil {
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

	if r.Header.Get("Git-Protocol") == "version=2" {
		comm := packp.NewCommandRequest()
		if err := comm.Decode(body); err != nil {
			logger.Error("command decode", err.Error())
			http.Error(w, "command decode: "+err.Error(), http.StatusBadRequest)
			return
		}
		logger.Debug("request:", comm.Command, "caps:", comm.Capabilities, "args:", comm.Args)

		res, err := sess.CommandHandler(r.Context(), comm)
		if err != nil {
			logger.Error("command", err.Error())
			http.Error(w, "command: "+err.Error(), http.StatusBadRequest)
			return
		}

		println("res is", res.Args)
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		err = res.Encode(w)
		if err != nil {
			logger.Error("command", err.Error())
		}
		return
	}

	upr := packp.NewUploadPackRequest()
	if err := upr.Decode(body); err != nil {
		logger.Error("upload decode", err.Error())
		http.Error(w, "upload decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	logger.Debug("request:", upr.Wants, upr.Haves, *upr.Capabilities)

	res, err := sess.UploadPack(r.Context(), upr)
	if err != nil {
		logger.Error("upload pack", err.Error())
		http.Error(w, "upload pack: "+err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Debug("response:", res.ACKs, res.ServerResponse.ACKs)

	buff := bytes.Buffer{}
	if err := res.Encode(&buff); err != nil {
		logger.Error("encode response", err.Error())
		http.Error(w, "encode response", http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Write(buff.Bytes())

	logger.Debug("sent", res.ServerResponse, res.ACKs)
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
