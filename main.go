package gwi

import (
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"blmayer.dev/git/gwi/internal/logger"

	"github.com/gorilla/mux"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/gomarkdown/markdown"
)

type User struct {
	Login string
	Pass  string
}

type File struct {
	*object.File
	Size int64
}

type RepoInfo struct {
	Name     string
	Ref      string
	Desc     string
	CloneURL string
	Creator  string
	Files    []File
	Owners   []string
	Commits  []*object.Commit
	Branches []*plumbing.Reference
	Readme   template.HTML
	License  template.HTML
}

type Config struct {
	Domain    string
	PagesRoot string
	Root      string
	CGIRoot   string
	CGIPrefix string
}

type Vault interface {
	Validate(login, pass string) bool
}

type Gwi struct {
	config Config
	pages   *template.Template
	handler *mux.Router
	vault   Vault
}

func NewFromConfig(config Config, vault Vault) (Gwi, error) {
	gwi := Gwi{config: config, vault: vault}

	r := mux.NewRouter()

	r.Handle("/", http.HandlerFunc(gwi.UserListHandler))
	r.Handle("/index.html", http.HandlerFunc(gwi.UserListHandler))
	r.Handle("/{user}/index.html", http.HandlerFunc(gwi.RepoListHandler))
	r.Handle("/{user}/{repo}", http.HandlerFunc(gwi.IndexHandler))
	r.Handle("/{user}/{repo}/branches", http.HandlerFunc(gwi.BranchesHandler))
	r.Handle("/{user}/{repo}/commits/{commit}", http.HandlerFunc(gwi.CommitHandler))
	r.Handle("/{user}/{repo}/{ref}/commits", http.HandlerFunc(gwi.CommitsHandler))
	r.Handle("/{user}/{repo}/{ref}/tree", http.HandlerFunc(gwi.TreeHandler))
	r.PathPrefix("/{user}/{repo}/{ref}/files/{file}").Handler(http.HandlerFunc(gwi.FileHandler))

	r.HandleFunc("/{user}/{repo}/info/{service}", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/git-receive-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/git-upload-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/objects/info", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/HEAD", gwi.GitCGIHandler)

	gwi.handler = r

	// read templates
	var err error
	logger.Debug("parsing templates...")
	gwi.pages, err = template.ParseGlob(path.Join(config.PagesRoot, "*.html"))

	return gwi, err
}

func (g *Gwi) Handle() http.Handler {
	return g.handler
}

func (g *Gwi) UserListHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("path:", r.URL.Path)

	dir, err := os.ReadDir(g.config.Root)
	if err != nil {
		logger.Debug("readDir error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var users []string
	for _, d := range dir {
		if !d.IsDir() {
			continue
		}

		users = append(users, d.Name())
	}

	w.Header().Set("Content-Type", "text/html")
	if err := g.pages.ExecuteTemplate(w, "users.html", users); err != nil {
		println("execute error:", err.Error())
	}
}

func (g *Gwi) RepoListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	logger.Debug("path:", r.URL.Path)
	user := mux.Vars(r)["user"]
	userDir := path.Join(g.config.Root, user)

	dir, err := os.ReadDir(userDir)
	if err != nil {
		logger.Debug("readDir error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var repos []RepoInfo
	for _, d := range dir {
		if !d.IsDir() {
			continue
		}
		r := RepoInfo{Name: d.Name()}
		r.Desc = readDesc(path.Join(userDir, r.Name))

		repos = append(repos, r)
	}

	if err := g.pages.ExecuteTemplate(w, "repos.html", repos); err != nil {
		println("execute error:", err.Error())
	}
}

func (g *Gwi) IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := RepoInfo{
		Commits: []*object.Commit{},
		Creator: mux.Vars(r)["user"],
		Name: mux.Vars(r)["repo"],
	}
	repoDir := path.Join(g.config.Root, info.Creator, info.Name)
	logger.Debug("repo:", info.Name)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// description
	info.Desc = readDesc(repoDir)
	info.CloneURL = "https://"+g.config.Domain+"/" + info.Name

	// files
	head, err := repo.Head()
	if err == plumbing.ErrReferenceNotFound {
		g.pages.ExecuteTemplate(w, "empty.html", info)
		return
	}
	if err != nil {
		logger.Error("head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	info.Ref = head.Hash().String()

	headObj, err := repo.CommitObject(head.Hash())
	if err != nil {
		logger.Error("head commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tree, err := headObj.Tree()
	if err != nil {
		logger.Error("trees error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tree.Files().ForEach(func(f *object.File) error {
		switch strings.ToLower(f.Name) {
		case "readme.md":
			if reader, err := f.Blob.Reader(); err == nil {
				readme, _ := io.ReadAll(reader)
				info.Readme = template.HTML(
					"<article>"+string(markdown.ToHTML(readme, nil, nil))+"</article>",
				)
			} else {
				logger.Debug("read readme error:", err.Error())
			}
		case "readme.txt", "readme":
			if reader, err := f.Blob.Reader(); err == nil {
				readme, _ := io.ReadAll(reader)
				info.Readme = template.HTML(
					"<pre>" + string(readme) + "</pre>",
				)
			} else {
				logger.Debug("read readme error:", err.Error())
			}
		case "license.md":
			if reader, err := f.Blob.Reader(); err == nil {
				lic, _ := io.ReadAll(reader)
				info.License = template.HTML(
					"<article>"+string(markdown.ToHTML(lic, nil, nil))+"</article>",
				)
			} else {
				logger.Debug("read license error:", err.Error())
			}
		case "license.txt", "license":
			if reader, err := f.Blob.Reader(); err == nil {
				license, _ := io.ReadAll(reader)
				info.License = template.HTML(
					"<pre>" + string(license) + "</pre>",
				)
			} else {
				logger.Debug("read license error:", err.Error())
			}
		}

		return nil
	})

	if err := g.pages.ExecuteTemplate(w, "summary.html", info); err != nil {
		logger.Error(err.Error())
	}
}

func (g *Gwi) BranchesHandler(w http.ResponseWriter, r *http.Request) {
	info := RepoInfo {
		Name:     mux.Vars(r)["repo"],
		Creator:  mux.Vars(r)["user"],
		Branches: []*plumbing.Reference{},
		CloneURL: "https://"+g.config.Domain+"/" + mux.Vars(r)["repo"],
	}
	repoDir := path.Join(g.config.Root, info.Creator, info.Name)
	logger.Debug("getting branches for repo", info.Name)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	head, err := repo.Head()
	if err == plumbing.ErrReferenceNotFound {
		g.pages.ExecuteTemplate(w, "empty.html", info)
		return
	}
	if err != nil {
		logger.Error("head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	info.Ref = head.Hash().String()

	// branches
	branches, err := repo.Branches()
	if err != nil {
		logger.Error("branches error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	branches.ForEach(func(b *plumbing.Reference) error {
		info.Branches = append(info.Branches, b)
		return nil
	})

	w.Header().Set("Content-Type", "text/html")
	if err := g.pages.ExecuteTemplate(w, "branches.html", info); err != nil {
		logger.Error(err.Error())
	}
}
