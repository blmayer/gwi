package gwi

import (
	"html/template"
	"net/http"
	"os"
	"path"
	"sort"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/gorilla/mux"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/vraycc/go-parsemail"

	"github.com/gomarkdown/markdown"
)

type ThreadStatus string

type User struct {
	Login string
	Pass  string
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

type Thread struct {
	Title string
	Creator string
	Lenght int
	Status ThreadStatus
}

type Config struct {
	Domain    string
	MailAddress string
	PagesRoot string
	Root      string
	CGIRoot   string
	CGIPrefix string
	LogLevel  logger.Level
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
	"head":     func() *plumbing.Reference { return nil },
	"thread":   func(section string) []Thread { return nil },
	"mails":    func(section string) []parsemail.Email { return nil },
	"desc":     func(ref plumbing.Hash) string { return "" },
	"branches": func(ref plumbing.Hash) []*plumbing.Reference { return nil },
	"tags":     func() []*plumbing.Reference { return nil },
	"commits":  func(ref plumbing.Hash) []*object.Commit { return nil },
	"commit":   func(ref plumbing.Hash) *object.Commit { return nil },
	"tree":     func(ref plumbing.Hash) []File { return nil },
	"file":     func(ref plumbing.Hash, name string) string { return "" },
	"markdown": func(in string) template.HTML { return template.HTML(markdown.ToHTML([]byte(in), nil, nil)) },
}

func NewFromConfig(config Config, vault Vault) (Gwi, error) {
	gwi := Gwi{
		config: config,
		vault: vault, 
		pages: template.New("all").Funcs(funcMapTempl),
	}

	if os.Getenv("DEBUG") != "" {
		logger.SetLevel(logger.DebugLevel)
	}

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
		"tags":     g.tags(repo),
		"commits":  g.commits(repo),
		"commit":   g.commit(repo),
		"tree":     g.tree(repo),
		"file":     g.file(repo),
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

func (g *Gwi) thread(user, repo string) func(section string) []Thread {
	return func(section string) []Thread {
		logger.Debug("getting threads for", section)

		dir, err := os.ReadDir(
			path.Join(g.config.Root, user, repo, "mail", section),
		)
		if err != nil {
			logger.Debug("readDir error:", err.Error())
			return nil
		}

		var threads []Thread
		for _, d := range dir {
			if !d.IsDir() {
				continue
			}

			t := Thread{Title: d.Name()}

			threads = append(threads, t)
		}

		return threads
	}
}

func (g *Gwi) mails(user, repo string) func(section, thread string) []parsemail.Email {
	return func(section, thread string) []parsemail.Email {
		logger.Debug("getting mail for", section, thread)

		dir := path.Join(g.config.Root, user, repo, "mail", section, thread)
		threadDir, err := os.ReadDir(dir)
		if err != nil {
			logger.Error("readDir error:", err.Error())
			return nil
		}

		var mail []parsemail.Email
		for _, t := range threadDir {
			logger.Debug("opening", t.Name())
			mailFile, err := os.Open(path.Join(dir, t.Name()))
			if err != nil {
				logger.Error("open mail error:", err.Error())
				continue
			}

			logger.Debug("parsing mail")
			m, err := parsemail.Parse(mailFile)
			if err != nil {
				logger.Error("email parse error:", err.Error())
			}
			logger.Debug("closing mail")
			if err := mailFile.Close(); err != nil {
				logger.Error("email close error:", err.Error())
			}

			mail = append(mail, m)
		}

		sort.Slice(
			mail,
			func(i, j int) bool { return mail[i].Date.Before(mail[j].Date) },
		)
		return mail
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

func (g *Gwi) head(ref *plumbing.Reference) func() *plumbing.Reference {
	return func() *plumbing.Reference {
		return ref
	}
}

func (g *Gwi) desc(repo *git.Repository) func(ref plumbing.Hash) string {
	return func(ref plumbing.Hash) string {
		logger.Debug("getting desc for ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			logger.Error("commitObject error:", err.Error())
			return ""
		}

		tree, err := commit.Tree()
		if err != nil {
			logger.Error("tree error:", err.Error())
			return ""
		}
		descFile, err := tree.File("DESC")
		if err != nil && err != object.ErrFileNotFound {
			logger.Error("descFile error:", err.Error())
			return ""
		}
		if err == object.ErrFileNotFound {
			return ""
		}

		content, err := descFile.Contents()
		if err != nil {
			logger.Error("contents error:", err.Error())
			return ""
		}

		return content
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

func (g *Gwi) tags(repo *git.Repository) func() []*plumbing.Reference {
	return func() []*plumbing.Reference {
		logger.Debug("getting tags")
		tgs, err := repo.Tags()
		if err != nil {
			logger.Error("tags error:", err.Error())
			return nil
		}

		var tags []*plumbing.Reference
		tgs.ForEach(func(t *plumbing.Reference) error {
			tags = append(tags, t)
			return nil
		})
		return tags
	}
}
