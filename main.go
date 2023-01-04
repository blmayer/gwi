// gwi stands for Git Web Interface, so it lets you customize the appearance
// of your git repositories using templates. gwi is intended to be run on
// servers where your bare git repositories are located, so it can detect
// and render them correctly.
//
// gwi works in a simple way: it is a web server, and your request's path
// points which user and repo are selected, i.e.:
//
//  GET root/user/repo/action/args
//
// selects the repository named repo from the user named user. Those are
// just hierarchical abstractions. Then the next folder in the path defines
// the template it will run, in this case the action, so gwi will execute
// a template named action.html with the selected repo information available.
// Lastly, everything that comes after action is part of args, and it is passed
// to templates under the Args field.
//
// # Template functions
//
// This package provides functions that you can call in your templates,
// letting you query the data you want in an efficient way. Currently we
// export the following functions:
//
//  usage   
//  users   
//  repos   
//  head    
//  thread  
//  mails   
//  desc    
//  branches
//  tags    
//  log 
//  commits 
//  commit  
//  tree    
//  files   
//  file
//  markdown
//
// Which can be called on templates using the standard template syntax.
//
// To see complete details about them see [FuncMapTempl].
//
// # Handlers
//
// gwi comes with 2 handlers: Main and List, which are meant to be used in
// different situations. See their respective docs for their use.
//
// # Example
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

// User interface represents what a user should provide at a minimum. This
// interface is available on templates and is also used internaly.
type User interface {
	Email() string
	Login() string
	Pass() string
}

type File struct {
	*object.File
	Size int64
}

// Info is the structure that is passed as data to templates being executed.
// The values are filled with the selected repo and user given on the URL.
type Info struct {
	User    string
	Repo    string
	Ref     plumbing.Hash
	RefName string
	Args    string
}

// Config is used to configure the gwi application, things like Root, PagesRoot
// and CGIRoot are the central part that make gwi work. Domain, MailAddress and
// Functions are mostly used to enhance the information displayed on templates.
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

// Vault is used to authenticate write calls to git repositories, the Vault
// implementation [FileVault] is a simple example that uses salt and hashes
// to store and validate users. In real applications you should use a better
// approache and implement your own Vault interface.
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

var FuncMapTempl = map[string]any{
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
	for name, f := range FuncMapTempl {
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

// ListHandler is used for listing users, or repos for a user given in the URL
// path, this handler is usefull for creating listings of projects, as this is
// very light on reads, and can be executed more often. It populates the
// template data with just User and Repo fields, along with 2 functions: users
// and repos.
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

// MainHandler is the handler used to display information about a repository.
// It contains all functions defined it [FuncMapTempl] with the correct user
// and repo selected; and provides the complete Info struct as data to the
// template. This handler is used to display data like commits, files, branches
// and tags about a given repo.
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

	// TODO: Improve ref name, as it changes between invocations
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
