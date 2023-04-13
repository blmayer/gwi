// gwi stands for Git Web Interface, so it lets you customize the appearance
// of your git repositories using templates. gwi is intended to be run on
// servers where your bare git repositories are located, so it can detect
// and render them correctly.
//
// gwi works in a simple way: it is a web server, and your request's path
// points which user and repo are selected, i.e.:
//
//	GET root/user/repo/action/args
//
// selects the repository named repo from the user named user. Those are
// just hierarchical abstractions. Then the next folder in the path defines
// the template it will run, in this case the action, so gwi will execute
// a template named action.html with the selected repo information available.
// Lastly, everything that comes after action is part of args, and it is passed
// to templates under the Args field.
//
// Some paths have special purposes and cannot be used by templates, they are:
//
//   - /user/repo/zip: for making archives
//   - /user/repo/info/refs: this and the following are used by git
//   - /user/repo/git-receive-pack
//   - /user/repo/git-upload-pack
//
// Creating template files with the names above will disable some features.
//
// # User authentication
//
// gwi currently only supports HTTP Basic flow, authorization/authentication
// is only needed in the git-recive-pack handler. For user validation this
// project provides the [Vault] interface, which you should implement. Consult
// the [FileVault] struct for an example.
//
// # Template functions
//
// This package provides functions that you can call in your templates,
// letting you query the data you want in an efficient way. Currently we
// export the following functions:
//
//   - usage
//   - users
//   - repos
//   - head
//   - thread
//   - mails
//   - markdown
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
// The default branch for git is main.
//
// # Examples
//
// The most simple way of using this is initializing and using the handle
// function:
//
//	package main
//
//	import (
//		"net/http"
//
//		"blmayer.dev/gwi"
//	)
//
//	func main() {
//		// init user vault
//		v, err := NewFileVault("users.json", "--salt--")
//		// handle error
//
//		// gwi config struct
//		c := gwi.Config{
//			Root: "path/to/git/folder",
//			PagesRoot: "path/to/html-templates",
//			...
//		}
//
//		g, _ := gwi.NewFromConfig(c, v)
//		// handle error
//
//		err := http.ListenAndServe(":8080", g.Handle())
//		// handle err
//	}
//
// Another good example is [main_test.go].
//
// Using templates provided:
//
//	Repo has {{commits .Ref}} commits.
//
// Will print the number of commits on the repo.
package gwi

import (
	"archive/zip"
	"html/template"
	"net/http"
	"os"
	"path"

	"blmayer.dev/x/dovel/interfaces/file"
	"blmayer.dev/x/gwi/internal/logger"

	"github.com/gorilla/mux"

	git "github.com/libgit2/git2go/v34"

	"github.com/microcosm-cc/bluemonday"
)

type Params map[string]any

func NewParams() Params {
	return map[string]any{}
}

func (p Params) Get(k string) any {
	return p[k]
}

func (p Params) Set(k string, v any) Params {
	p[k] = v
	return p
}

// User interface represents what a user should provide at a minimum. This
// interface is available on templates and is also used internaly.
type User interface {
	Email() string
	Login() string
	Pass() string
}

// Info is the structure that is passed as data to templates being executed.
// The values are filled with the selected repo and user given on the URL.
type Info struct {
	User   string
	Repo   string
	Args   string
	Params Params
	Git    *git.Repository
}

// Config is used to configure the gwi application, things like Root and
// PagesRoot are the central part that make gwi work. Domain, MailAddress and
// Functions are mostly used to enhance the information displayed on templates.
type Config struct {
	Domain      string
	MailAddress string
	PagesRoot   string
	Root        string
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

// GWI is the git instance, it exports the handlers that are used to handle
// git requests
type Gwi struct {
	config    Config
	pages     *template.Template
	handler   *mux.Router
	vault     Vault
	mailer    file.FileHandler
	functions map[string]func(params ...any) any
}

var p = bluemonday.UGCPolicy()

// FuncMapTempl gives the signatures for all functions available on templates.
var FuncMapTempl = map[string]any{
	// "sysinfo":  sysInfo,
	"usage":    diskUsage,
	"users":    func() []string { return nil },
	"repos":    func() []Info { return nil },
	"threads":  func(section string) []any { return nil },
	"mails":    func(thread string) []any { return nil },
	"markdown": mdown,
	"iter":     iter,
	"seq":      seq,
}

func NewFromConfig(config Config, vault Vault) (Gwi, error) {
	gwi := Gwi{
		config: config,
		vault:  vault,
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

	// mail
	var err error
	gwi.mailer, err = file.NewFileHandler(
		file.FileConfig{Root: config.Root},
		funcMap,
	)
	if err != nil {
		logger.Error("new mailer error", err.Error())
	}

	r := mux.NewRouter()
	r.HandleFunc("/{user}/{repo}/info/refs", gwi.infoRefsHandler).
		Queries("service", "{service}")
	r.HandleFunc("/{user}/{repo}/git-receive-pack", gwi.receivePackHandler)
	r.HandleFunc("/{user}/{repo}/git-upload-pack", gwi.uploadPackHandler)
	r.HandleFunc("/{user}/{repo}/HEAD", gwi.headHandler)
	r.HandleFunc("/{user}/{repo}/objects/{pre:.{2}}/{obj:.+}", gwi.objHandler)
	r.HandleFunc("/{user}/{repo}/objects/{obj:.+}", gwi.fileHandler)

	r.HandleFunc("/", gwi.ListHandler)
	r.HandleFunc("/{user}", gwi.ListHandler)
	r.HandleFunc("/{user}/{repo}/zip", gwi.zipHandler)
	r.HandleFunc("/{user}/{repo}/{op}/{args:.*}", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}/{op}/{args:.*}", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}/{op}", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}/", gwi.MainHandler)
	r.HandleFunc("/{user}/{repo}", gwi.MainHandler)

	gwi.handler = r

	// read templates
	logger.Debug("parsing templates...")
	gwi.pages, err = gwi.pages.ParseGlob(path.Join(config.PagesRoot, "*.html"))

	return gwi, err
}

// Handle returns all handlers defined here, it should be used to handle
// requests, as this provides the list and main handlers in the correct path.
func (g *Gwi) Handle() http.Handler {
	return g.handler
}

// ListHandler is used for listing users, or repos for a user given in the URL
// path, this handler is useful for creating listings of projects, as this is
// very light on reads, and can be executed more often. It populates the
// template data with just User and Repo fields, along with 2 functions: users
// and repos.
func (g *Gwi) ListHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	logger.Debug("running list handler with", vars)

	funcMap := map[string]any{
		"users": g.users(),
	}

	w.Header().Set("Content-Type", "text/html")
	page := "users.html"
	if vars["user"] != "" {
		funcMap["repos"] = g.repos(vars["user"])
		page = "repos.html"
	}

	pages := g.pages.Funcs(funcMap)
	if err := pages.ExecuteTemplate(w, page, nil); err != nil {
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
	ps := NewParams()
	for k, v := range r.URL.Query() {
		ps.Set(k, v[0])
	}

	info := Info{
		User:   vars["user"],
		Repo:   vars["repo"],
		Args:   vars["args"],
		Params: ps,
	}

	var err error
	repoDir := path.Join(g.config.Root, vars["user"], vars["repo"])
	info.Git, err = git.OpenRepository(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	funcMap := map[string]any{
		"threads": g.threads(repoDir),
		"mails":   g.mails(repoDir),
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

func (g *Gwi) zipHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	logger.Debug("running zip handler with", vars)

	user := vars["user"]
	repo := vars["repo"]
	repoDir := path.Join(g.config.Root, user, repo)

	repoGit, err := git.OpenRepository(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ref, err := git.NewOid(r.URL.Query().Get("ref"))
	if err != nil {
		head, err := repoGit.Head()
		if err != nil {
			logger.Error("git head error:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ref = head.Target()
	}
	commit, err := repoGit.LookupCommit(ref)
	if err != nil {
		logger.Error("commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Debug("getting tree for commit", commit.Id().String())
	tree, err := commit.Tree()
	if err != nil {
		logger.Error("trees error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	arc := zip.NewWriter(w)
	tree.Walk(func(name string, entry *git.TreeEntry) error {
		logger.Debug("getting", name)
		z, err := arc.Create(name)
		if err != nil {
			logger.Error("create file error:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return err
		}

		content, err := repoGit.LookupBlob(entry.Id)
		if err != nil {
			logger.Error("content error:", err.Error())
			return err
		}

		_, err = z.Write(content.Contents())
		if err != nil {
			logger.Error("write file error:", err.Error())
			return err
		}
		return nil
	})

	err = arc.Close()
	if err != nil {
		logger.Error("close file error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
