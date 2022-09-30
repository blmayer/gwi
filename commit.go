package gwi

import (
	"net/http"
	"path"

	"blmayer.dev/git/gwi/internal/logger"

	"github.com/gorilla/mux"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitInfo struct {
	Repo   string
	Commit *object.Commit
	Patch  string
}

func (g *Gwi) CommitHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	hash := mux.Vars(r)["commit"]
	repoName := mux.Vars(r)["repo"]
	logger.Debug("commit:", hash)

	repo, err := git.PlainOpen(path.Join(g.gitRoot, repoName))
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	commitObj, err := repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		logger.Error("head commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tree, err := commitObj.Tree()
	if err != nil {
		logger.Error("commit tree error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	info := CommitInfo{Repo: repoName, Commit: commitObj}
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

	if err := g.pages.ExecuteTemplate(w, "commit.html", info); err != nil {
		logger.Error(err.Error())
	}
}

func (g *Gwi) CommitsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := struct {
		Name     string
		Desc     string
		CloneURL string
		Commits  []*object.Commit
	}{
		Name:    mux.Vars(r)["repo"],
		Commits: []*object.Commit{},
	}
	logger.Debug("getting commits for repo", info.Name)

	repo, err := git.PlainOpen(path.Join(g.gitRoot, info.Name))
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// get commits
	logs, err := repo.Log(&git.LogOptions{})
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
