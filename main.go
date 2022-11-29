package gwi

import (
	"html/template"
	"net/http"
	"os"
	"path"

	"blmayer.dev/x/gwi/internal/logger"

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

type Info struct {
	User     string
	Repo     string
	Ref      plumbing.Hash
	RefName  string
	Args     string
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
	config  Config
	pages   *template.Template
	handler *mux.Router
	vault   Vault
}

var funcMapTempl = map[string]any{
	"users":    func() []string { return nil },
	"repos":    func(user string) []string { return nil },
	"commits":  func(ref plumbing.Hash) []*object.Commit { return nil },
	"commit":   func(ref plumbing.Hash) *object.Commit { return nil },
	"branches": func(ref plumbing.Hash) []*plumbing.Reference { return nil },
	"tree":     func(ref plumbing.Hash) []File { return nil },
	"file":     func(ref plumbing.Hash, name string) string { return "" },
	"markdown": func(in string) template.HTML { return template.HTML(markdown.ToHTML([]byte(in), nil, nil)) },
}

func NewFromConfig(config Config, vault Vault) (Gwi, error) {
	gwi := Gwi{config: config, vault: vault}

	r := mux.NewRouter()

	r.HandleFunc("/", gwi.ListHandler)
	r.HandleFunc("/{user}", gwi.ListHandler)
	r.HandleFunc("/{user}/{repo}/{op}/{ref:.*}", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}/{op}/{ref}/{args:.*}", gwi.MainHandler)

	r.HandleFunc("/{user}/{repo}/info/{service}", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/git-receive-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/git-upload-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/objects/info", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/HEAD", gwi.GitCGIHandler)

	gwi.handler = r

	// read templates
	var err error
	logger.Debug("parsing templates...")
	gwi.pages, err = template.New("all").Funcs(funcMapTempl).ParseGlob(path.Join(config.PagesRoot, "*.html"))

	return gwi, err
}

func (g *Gwi) Handle() http.Handler {
	return g.handler
}

func (g *Gwi) ListHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	logger.Debug("running list handler with", vars)

	info := Info{
		User: vars["user"],
		Repo: vars["repo"],
	}

	funcMap := map[string]any{
		"users": g.users(),
		"repos": g.repos(),
	}
	pages := g.pages.Funcs(funcMap)

	w.Header().Set("Content-Type", "text/html")
	page := "users.html"
	if info.User != "" {
		page = "repos.html"
	}

	if err := pages.ExecuteTemplate(w, page, info); err != nil {
		logger.Error("execute error:", err.Error())
	}
}

func (g *Gwi) MainHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	logger.Debug("running main handler with", vars)

	info := Info{
		User: vars["user"],
		Repo: vars["repo"],
		Ref:  plumbing.NewHash(vars["ref"]),
		Args: vars["args"],
	}
	repoDir := path.Join(g.config.Root, info.User, info.Repo)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if vars["ref"] == "" {
		head, _ := repo.Head()
		info.Ref = head.Hash()
		info.RefName = head.Name().Short()
	}

	funcMap := map[string]any{
		"users": g.users(),
		"repos": g.repos(),
		"branches": g.branches(repo),
		"commits":  g.commits(repo),
		"commit":   g.commit(repo),
		"tree":     g.tree(repo),
		"file":     g.file(repo),
	}
	pages := g.pages.Funcs(funcMap)

	w.Header().Set("Content-Type", "text/html")
	if err := pages.ExecuteTemplate(w, vars["op"]+".html", info); err != nil {
		logger.Error("execute error:", err.Error())
	}
}

func (g *Gwi) users() func() []string {
	return func() []string {
		logger.Debug("getting users")
		dir, err := os.ReadDir(g.config.Root)
		if err != nil {
			logger.Debug("readDir error:", err.Error())
			return nil
		}

		var users []string
		for _, d := range dir {
			if !d.IsDir() {
				continue
			}

			users = append(users, d.Name())
		}

		return users
	}
}

func (g *Gwi) repos() func(user string) []string {
	return func(user string) []string {
		logger.Debug("getting repos for", user)
		dir, err := os.ReadDir(path.Join(g.config.Root, user))
		if err != nil {
			logger.Debug("readDir error:", err.Error())
			return nil
		}

		var rs []string
		for _, d := range dir {
			if !d.IsDir() {
				continue
			}

			rs = append(rs, d.Name())
		}

		return rs
	}
}

func (g *Gwi) branches(repo *git.Repository) func(ref plumbing.Hash) []*plumbing.Reference {
	return func(ref plumbing.Hash) []*plumbing.Reference {
		logger.Debug("getting branches for ref", ref.String())
		brs, err := repo.Branches()
		if err != nil {
			logger.Error("branches error:", err.Error())
			return nil
		}

		var branches []*plumbing.Reference
		brs.ForEach(func(b *plumbing.Reference) error {
			branches = append(branches, b)
			return nil
		})
		return branches
	}
}

