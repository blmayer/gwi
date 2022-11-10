package gwi

import (
	"net/http"
	"path"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/gorilla/mux"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitInfo struct {
	Creator  string
	Name     string
	Desc     string
	CloneURL string
	Ref      string
	Commit   *object.Commit
	Patch    string
}

func (g *Gwi) CommitHandler(w http.ResponseWriter, r *http.Request) {
	info := CommitInfo{
		Name:     mux.Vars(r)["repo"],
		Creator:  mux.Vars(r)["user"],
		Ref:      mux.Vars(r)["commit"],
		CloneURL: "https://" + g.config.Domain + "/" + mux.Vars(r)["repo"],
	}
	logger.Debug("commit:", info.Ref)
	repoDir := path.Join(g.config.Root, info.Creator, info.Name)
	info.Desc = readDesc(repoDir)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	commitObj, err := repo.CommitObject(plumbing.NewHash(info.Ref))
	if err != nil {
		logger.Error("head commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	info.Commit = commitObj

	tree, err := commitObj.Tree()
	if err != nil {
		logger.Error("commit tree error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var parTree *object.Tree
	if par, err := commitObj.Parent(0); err == nil {
		parTree, _ = par.Tree()
	}

	patch, err := parTree.Patch(tree)
	if err != nil {
		logger.Error("commit patch error:", err.Error())
	} else {
		info.Patch = patch.String()
	}

	w.Header().Set("Content-Type", "text/html")
	if err := g.pages.ExecuteTemplate(w, "commit.html", info); err != nil {
		logger.Error(err.Error())
	}
}

func (g *Gwi) CommitsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := RepoInfo{
		Name:     mux.Vars(r)["repo"],
		Creator:  mux.Vars(r)["user"],
		Ref:      mux.Vars(r)["ref"],
		Commits:  []*object.Commit{},
		CloneURL: "https://" + g.config.Domain + "/" + mux.Vars(r)["repo"],
	}
	logger.Debug("getting commits for repo", info.Name)
	repoDir := path.Join(g.config.Root, info.Creator, info.Name)
	info.Desc = readDesc(repoDir)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// get commits
	logs, err := repo.Log(&git.LogOptions{From: plumbing.NewHash(info.Ref)})
	if err != nil {
		logger.Error("git log error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for count := 200; count > 0; count-- {
		c, _ := logs.Next()

		if c == nil {
			break
		}
		info.Commits = append(info.Commits, c)
	}

	if err := g.pages.ExecuteTemplate(w, "log.html", info); err != nil {
		logger.Error(err.Error())
	}
}
