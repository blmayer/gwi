package gwi

import (
	"html/template"
	"net/http"
	"os"
	"path"

	"blmayer.dev/x/dovel/interfaces/file"
	"blmayer.dev/x/gwi/internal/logger"

	"github.com/gorilla/mux"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/microcosm-cc/bluemonday"
)

type ThreadStatus string

type User interface {
	Email() string
	Login() string
	Pass() string
}

type File struct {
	*object.File
	Size int64
}

type Info struct {
	User    string
	Repo    string
	Ref     plumbing.Hash
	RefName string
	Args    string
}

type Config struct {
	Domain      string
	MailAddress string
	PagesRoot   string
	Root        string
	CGIRoot     string
	CGIPrefix   string
	LogLevel    logger.Level
	Functions   map[string]func(p ...any) any
}

type Vault interface {
	GetUser(login string) User
	Validate(login, pass string) bool
}

type Gwi struct {
	config    Config
	pages     *template.Template
	handler   *mux.Router
	vault     Vault
	mailer    file.FileConfig
	functions map[string]func(params ...any) any
}

var p = bluemonday.UGCPolicy()

var funcMapTempl = map[string]any{
	// "sysinfo":  sysInfo,
	"usage":    diskUsage,
	"users":    func() []string { return nil },
	"repos":    func(user string) []string { return nil },
	"head":     func() *plumbing.Reference { return nil },
	"thread":   func(section string) []any { return nil },
	"mails":    func(thread string) []any { return nil },
	"desc":     func(ref plumbing.Hash) string { return "" },
	"branches": func(ref plumbing.Hash) []*plumbing.Reference { return nil },
	"tags":     func() []*plumbing.Reference { return nil },
	"log":  func(ref plumbing.Hash) []*object.Commit { return nil },
	"commits":  func(ref plumbing.Hash) int { return -1 },
	"commit":   func(ref plumbing.Hash) *object.Commit { return nil },
	"tree":     func(ref plumbing.Hash) []File { return nil },
	"files":     func(ref plumbing.Hash) int { return -1 },
	"file":     func(ref plumbing.Hash, name string) string { return "" },
	"markdown": mdown,
}

func NewFromConfig(config Config, vault Vault) (Gwi, error) {
	gwi := Gwi{
		config: config,
		vault:  vault,
		mailer: file.FileConfig{Root: config.Root},
	}

	if os.Getenv("DEBUG") != "" {
		logger.SetLevel(logger.DebugLevel)
	}

	// load functions
	funcMap := map[string]any{}
	for name, f := range funcMapTempl {
		funcMap[name] = f
	}
	for name, f := range config.Functions {
		funcMap[name] = f
	}
	gwi.pages = template.New("all").Funcs(funcMap)

	r := mux.NewRouter()
	r.HandleFunc("/{user}/{repo}/info/{service}", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/git-receive-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/git-upload-pack", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/objects/info", gwi.GitCGIHandler)
	r.HandleFunc("/{user}/{repo}/HEAD", gwi.GitCGIHandler)

	r.HandleFunc("/", gwi.ListHandler)
	r.HandleFunc("/{user}", gwi.ListHandler)
	r.HandleFunc("/{user}/{repo}/{op}/{args:.*}", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}/{op}", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}/", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}", gwi.MainHandler)

	gwi.handler = r

	// read templates
	var err error
	logger.Debug("parsing templates...")
	gwi.pages, err = gwi.pages.ParseGlob(path.Join(config.PagesRoot, "*.html"))

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
		Ref:  plumbing.NewHash(r.URL.Query().Get("ref")),
		Args: vars["args"],
	}
	repoDir := path.Join(g.config.Root, info.User, info.Repo)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	refIter, err := repo.References()
	if err != nil {
		logger.Error("git references error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	refs := map[plumbing.Hash]*plumbing.Reference{}
	refIter.ForEach(func(r *plumbing.Reference) error {
		refs[r.Hash()] = r
		return nil
	})

	head, err := repo.Head()
	if err != nil {
		logger.Error("git head error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("ref") == "" {
		info.Ref = head.Hash()
		info.RefName = head.Name().Short()
	} else {
		if refName, ok := refs[info.Ref]; ok {
			info.RefName = refName.Name().Short()
		} else {
			info.RefName = info.Ref.String()
		}
	}

	funcMap := map[string]any{
		"users":    g.users(),
		"repos":    g.repos(),
		"head":     g.head(head),
		"desc":     g.desc(repo),
		"thread":   g.thread(info.User, info.Repo),
		"mails":    g.mails(info.User, info.Repo),
		"branches": g.branches(repo),
		"tags": g.tags(repo),
		"log": g.log(repo),
		"commits": g.commits(repo),
		"commit": g.commit(repo),
		"tree": g.tree(repo),
		"files": g.files(repo),
		"file": g.file(repo),
	}
	pages := g.pages.Funcs(funcMap)

	op := vars["op"]
	if op == "" {
		op = "summary"
	}

	w.Header().Set("Content-Type", "text/html")
	if err := pages.ExecuteTemplate(w, op+".html", info); err != nil {
		logger.Error("execute error:", err.Error())
	}
}
