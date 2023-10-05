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
//   - desc
//   - branches
//   - tags
//   - log
//   - commits
//   - commit
//   - tree
//   - files
//   - file
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

	"log/slog"

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
	Git     *git.Repository
}

// Config is used to configure the gwi application, things like Root and
// PagesRoot are the central part that make gwi work. Domain, MailAddress and
// Functions are mostly used to enhance the information displayed on templates.
type Config struct {
	Domain      string
	MailAddress string
	PagesRoot   string
	Root        string
	LogLevel    slog.Level
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
	functions map[string]func(params ...any) any
}

var p = bluemonday.UGCPolicy()

// FuncMapTempl gives the signatures for all functions available on templates.
var FuncMapTempl = map[string]any{
	// "sysinfo":  sysInfo,
	"usage":    diskUsage,
	"users":    func() []string { return nil },
	"repos":    func(user string) []string { return nil },
	"head":     func() *plumbing.Reference { return nil },
	"threads":  func(section string) []any { return nil },
	"mails":    func(thread string) []any { return nil },
	"desc":     func(ref plumbing.Hash) string { return "" },
	"branches": func(ref plumbing.Hash) []*plumbing.Reference { return nil },
	"tags":     func() []*plumbing.Reference { return nil },
	"log":      func(ref plumbing.Hash) []*object.Commit { return nil },
	"commits":  func(ref plumbing.Hash) int { return -1 },
	"commit":   func(ref plumbing.Hash) *object.Commit { return nil },
	"tree":     func(ref plumbing.Hash) []File { return nil },
	"files":    func(ref plumbing.Hash) int { return -1 },
	"file":     func(ref plumbing.Hash, name string) string { return "" },
	"markdown": mdown,
	"wrap":     wrap,
}

func NewFromConfig(cfg Config, vault Vault) (Gwi, error) {
	gwi := Gwi{
		config: cfg,
		vault:  vault,
	}

	if os.Getenv("DEBUG") != "" {
		hand := slog.NewTextHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: slog.LevelDebug},
		)
		slog.SetDefault(slog.New(hand))
	}

	// load functions
	funcMap := map[string]any{}
	for name, f := range FuncMapTempl {
		funcMap[name] = f
	}
	for name, f := range cfg.Functions {
		funcMap[name] = f
	}
	gwi.pages = template.New("all").Funcs(funcMap).Option()

	// mail
	var err error
	if err != nil {
		slog.Error("new mailer", "error", err.Error())
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
	slog.Debug("parsing templates...")
	gwi.pages, err = gwi.pages.ParseGlob(path.Join(cfg.PagesRoot, "*.html"))

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
	slog.Debug("running list handler with", "vars", vars)

	w.Header().Set("Content-Type", "text/html")

	info := struct {
		Users []string
		Repos []Info
	}{}

	page := "users.html"
	if user := vars["user"]; user == "" {
		slog.Debug("getting users")
		dir, err := os.ReadDir(g.config.Root)
		if err != nil {
			slog.Debug("readDir", "error", err.Error())
		}

		for _, d := range dir {
			if !d.IsDir() {
				continue
			}

			info.Users = append(info.Users, d.Name())
		}
	} else {
		slog.Debug("getting repos", "user", user)
		page = "repos.html"

		root := path.Join(g.config.Root, user)
		dir, err := os.ReadDir(root)
		if err != nil {
			slog.Debug("readDir", "error", err.Error())
		}

		for _, d := range dir {
			if !d.IsDir() {
				continue
			}

			repo, err := git.PlainOpen(path.Join(root, d.Name()))
			if err != nil {
				slog.Debug("open repo", "error", err.Error())
				continue
			}
			info.Repos = append(info.Repos, Info{Repo: d.Name(), Git: repo})
		}
	}

	if err := g.pages.ExecuteTemplate(w, page, info); err != nil {
		slog.Error("execute", "error", err.Error())
	}
}

// MainHandler is the handler used to display information about a repository.
// It contains all functions defined it [FuncMapTempl] with the correct user
// and repo selected; and provides the complete Info struct as data to the
// template. This handler is used to display data like commits, files, branches
// and tags about a given repo.
func (g *Gwi) MainHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slog.Debug("running main handler", "vars", vars)

	info := Info{
		User: vars["user"],
		Repo: vars["repo"],
		Ref:  plumbing.NewHash(r.URL.Query().Get("ref")),
		Args: vars["args"],
	}
	repoDir := path.Join(g.config.Root, info.User, info.Repo)

	var err error
	info.Git, err = git.PlainOpen(repoDir)
	if err != nil {
		slog.Error("git PlainOpen", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	funcMap := map[string]any{
	}
	pages := g.pages.Funcs(funcMap)

	op := vars["op"]
	if op == "" {
		op = "summary"
	}

	w.Header().Set("Content-Type", "text/html")
	if err := pages.ExecuteTemplate(w, op+".html", info); err != nil {
		slog.Error("execute", "error", err.Error())
	}
}

func (g *Gwi) zipHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slog.Debug("running zip handler", "vars", vars)

	info := Info{
		User: vars["user"],
		Repo: vars["repo"],
		Ref:  plumbing.NewHash(r.URL.Query().Get("ref")),
	}
	repoDir := path.Join(g.config.Root, info.User, info.Repo)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		slog.Error("git PlainOpen", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("ref") == "" {
		head, err := repo.Head()
		if err != nil {
			slog.Error("git head", "error", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		info.Ref = head.Hash()
	}
	commit, err := repo.CommitObject(info.Ref)
	if err != nil {
		slog.Error("commit", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Debug("getting tree for commit", "hash", commit.Hash.String())
	tree, err := commit.Tree()
	if err != nil {
		slog.Error("trees", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	arc := zip.NewWriter(w)
	tree.Files().ForEach(func(f *object.File) error {
		slog.Debug("getting file", "name", f.Name)
		z, err := arc.Create(f.Name)
		if err != nil {
			slog.Error("create file", "error", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return err
		}

		content, err := f.Contents()
		if err != nil {
			slog.Error("content", "error", err.Error())
			return err
		}

		_, err = z.Write([]byte(content))
		if err != nil {
			slog.Error("write file", "error", err.Error())
			return err
		}
		return nil
	})

	err = arc.Close()
	if err != nil {
		slog.Error("close file", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
