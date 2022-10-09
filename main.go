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
)

type File struct {
	*object.File
	Size int64
}

type RepoInfo struct {
	Name     string
	Desc     string
	CloneURL string
	Creator  string
	Files    []File
	Owners   []string
	Commits  []*object.Commit
	Readme   []byte
	License  []byte
}

type Gwi struct {
	domain string
	pages   *template.Template
	root    string
	gitRoot string
	gitCgiRoot string
	cgiPrefix string
	handler *mux.Router
}

func NewGWI(root, gitRoot, gitCgiRoot, cgiPrefix, domain string) (Gwi, error) {
	gwi := Gwi{
		root: root, 
		gitRoot: gitRoot, 
		gitCgiRoot: gitCgiRoot, 
		cgiPrefix: cgiPrefix, 
		domain: domain,
	}

	r := mux.NewRouter()

	r.Handle("/", http.HandlerFunc(gwi.RepoListHandler))
	r.Handle("/index.html", http.HandlerFunc(gwi.RepoListHandler))
	r.Handle("/{repo}", http.HandlerFunc(gwi.IndexHandler))
	r.Handle("/{repo}/tree", http.HandlerFunc(gwi.TreeHandler))
	r.Handle("/{repo}/branches", http.HandlerFunc(gwi.BranchesHandler))
	r.Handle("/{repo}/commits", http.HandlerFunc(gwi.CommitsHandler))
	r.Handle("/{repo}/commits/{commit}", http.HandlerFunc(gwi.CommitHandler))
	r.PathPrefix("/{repo}/tree/{file}").Handler(http.HandlerFunc(gwi.FileHandler))

	r.HandleFunc("/{repo}/info/{service}", gwi.GitCGIHandler)
	r.HandleFunc("/{repo}/git-receive-pack", private(gwi.GitCGIHandler))
	r.HandleFunc("/{repo}/git-upload-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{repo}/objects/info", gwi.GitCGIHandler)
	r.HandleFunc("/{repo}/HEAD", gwi.GitCGIHandler)

	gwi.handler = r

	// read templates
	var err error
	logger.Debug("parsing templates...")
	gwi.pages, err = template.ParseGlob(path.Join(root, "*.html"))

	return gwi, err
}

func (g *Gwi) Handle() http.Handler {
	return g.handler
}

func (g *Gwi) RepoListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	logger.Debug("path:", r.URL.Path)

	dir, err := os.ReadDir(g.gitRoot)
	if err != nil {
		logger.Debug("readDir error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var repos []string
	for _, d := range dir {
		if d.IsDir() && d.Name()[0] != '.' {
			repos = append(repos, d.Name())
		}
	}
	logger.Debug(g.pages)
	if err := g.pages.ExecuteTemplate(w, "listing.html", repos); err != nil {
		println("execute error:", err.Error())
	}
}

func (g *Gwi) IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := RepoInfo{Commits: []*object.Commit{}}
	info.Name = r.URL.Path[1:]
	repoDir := path.Join(g.gitRoot, info.Name)
	logger.Debug("repo:", info.Name)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// description
	descBytes, err := os.ReadFile(path.Join(repoDir, "description"))
	if err != nil {
		logger.Error("read desc error:", err.Error())
	}
	info.Desc = string(descBytes)
	info.CloneURL = "https://"+g.domain+"/" + info.Name

	// files
	head, err := repo.Head()
	if err != nil {
		logger.Error("head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
		case "readme.md", "readme.txt", "readme":
			if reader, err := f.Blob.Reader(); err == nil {
				info.Readme, _ = io.ReadAll(reader)
			} else {
				logger.Debug("read readme error:", err.Error())
			}
		case "license.md", "license.txt", "license":
			if reader, err := f.Blob.Reader(); err == nil {
				info.License, _ = io.ReadAll(reader)
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

func (g *Gwi) FileHandler(w http.ResponseWriter, r *http.Request) {
	logger.Debug("file:", r.URL.Path)
	parts := strings.Split(r.URL.Path[1:], "/")

	repo, err := git.PlainOpen(path.Join(g.gitRoot, parts[0]))
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	head, err := repo.Head()
	if err != nil {
		logger.Error("head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	headObj, err := repo.CommitObject(head.Hash())
	if err != nil {
		logger.Error("head commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Debug("getting file", path.Join(parts[2:]...))
	file, err := headObj.File(path.Join(parts[2:]...))
	if err != nil {
		logger.Error("head file error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	content, _ := file.Contents()
	w.Write([]byte(content))
}

func (g *Gwi) TreeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := RepoInfo{}
	info.Name = mux.Vars(r)["repo"]
	logger.Debug("tree:", info.Name)

	repo, err := git.PlainOpen(path.Join(g.gitRoot, info.Name))
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// files
	head, err := repo.Head()
	if err != nil {
		logger.Error("head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
		size, _ := tree.Size(f.Name)
		info.Files = append(
			info.Files,
			File{
				File: f,
				Size: size,
			},
		)
		return nil
	})

	if err := g.pages.ExecuteTemplate(w, "tree.html", info); err != nil {
		logger.Error(err.Error())
	}
}

func (g *Gwi) BranchesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := struct {
		Name     string
		Desc     string
		CloneURL string
		Branches []*plumbing.Reference
	}{
		Name:     mux.Vars(r)["repo"],
		Branches: []*plumbing.Reference{},
	}
	logger.Debug("getting branches for repo", info.Name)

	repo, err := git.PlainOpen(path.Join(g.gitRoot, info.Name))
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	if err := g.pages.ExecuteTemplate(w, "branches.html", info); err != nil {
		logger.Error(err.Error())
	}
}
