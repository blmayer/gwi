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

func (g *Gwi) FileHandler(w http.ResponseWriter, r *http.Request) {
	user := mux.Vars(r)["user"]
	name := mux.Vars(r)["repo"]
	file := mux.Vars(r)["file"]
	ref := mux.Vars(r)["ref"]
	logger.Debug("file:", r.URL.Path, "at ref", ref)

	repo, err := git.PlainOpen(path.Join(g.config.Root, user, name))
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	headObj, err := repo.CommitObject(plumbing.NewHash(ref))
	if err != nil {
		logger.Error("head commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Debug("getting file", file)
	fileObj, err := headObj.File(file)
	if err != nil {
		logger.Error("head file error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	content, _ := fileObj.Contents()
	w.Write([]byte(content))
}

func (g *Gwi) file(repo *git.Repository) func(ref plumbing.Hash, name string) string {
	return func(ref plumbing.Hash, name string) string {

		logger.Debug("getting commit for ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			logger.Error("commit error:", err.Error())
			return nil
		}

		logger.Debug("getting file", file)
		fileObj, err := commit.File(file)
		if err != nil {
			logger.Error("head file error:", err.Error())
			return ""
		}
		return fileObject.Contents()
	}
}

func (g *Gwi) TreeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	info := RepoInfo{
		Creator: mux.Vars(r)["user"],
		Name:    mux.Vars(r)["repo"],
		Ref:     plumbing.NewHash(mux.Vars(r)["ref"]),
	}
	logger.Debug("tree:", info.Name)
	repoDir := path.Join(g.config.Root, info.Creator, info.Name)
	info.Desc = readDesc(repoDir)
	info.CloneURL = "https://" + path.Join(g.config.Domain, info.Creator, info.Name)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		logger.Error("git PlainOpen error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// files
	logger.Debug("getting tree for ref", info.Ref.String())
	commit, err := repo.CommitObject(info.Ref)
	if err != nil {
		logger.Error("commit error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tree, err := commit.Tree()
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

func (g *Gwi) tree(repo *git.Repository) func(ref plumbing.Hash) []File {
	return func(ref plumbing.Hash) []File {

		// files
		logger.Debug("getting commit for ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			logger.Error("commit error:", err.Error())
			return nil
		}
		tree, err := commit.Tree()
		if err != nil {
			logger.Error("trees error:", err.Error())
			return nil
		}

		var files []File
		tree.Files().ForEach(func(f *object.File) error {
			size, _ := tree.Size(f.Name)
			files = append(
				files,
				File{
					File: f,
					Size: size,
				},
			)
			return nil
		})
		return files
	}
}
