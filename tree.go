package gwi

import (
	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *Gwi) file(repo *git.Repository) func(ref plumbing.Hash, name string) string {
	return func(ref plumbing.Hash, name string) string {
		logger.Debug("getting commit for ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			logger.Error("commit error:", err.Error())
			return ""
		}

		logger.Debug("getting file", name)
		fileObj, err := commit.File(name)
		if err != nil {
			logger.Error(name, "file error:", err.Error())
			return ""
		}

		c, err := fileObj.Contents()
		if err != nil {
			logger.Error(name, "contents error:", err.Error())
			return ""
		}

		return c
	}
}

func (g *Gwi) files(repo *git.Repository) func(ref plumbing.Hash) int {
	return func(ref plumbing.Hash) int {
		// files
		logger.Debug("getting commit for ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			logger.Error("commit error:", err.Error())
			return -1
		}

		logger.Debug("getting files for commit", commit.Hash.String())
		t, err := commit.Tree()
		if err != nil {
			logger.Error("trees error:", err.Error())
			return -1
		}

		return countFiles(t)
	}
}

func countFiles(t *object.Tree) int {
	count := 0
	for _, e := range t.Entries {
		if e.Mode.IsFile() {
			count += 1
		} else {
			t2, err := t.Tree(e.Name)
			if err != nil {
				logger.Error("tree tree", err.Error())
				continue
			}
			count += countFiles(t2)
		}
	}
	return count
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

		logger.Debug("getting tree for commit", commit.Hash.String())
		tree, err := commit.Tree()
		if err != nil {
			logger.Error("trees error:", err.Error())
			return nil
		}

		var files []File
		tree.Files().ForEach(func(f *object.File) error {
			logger.Debug("getting", f.Name)
			size, _ := tree.Size(f.Name)
			files = append(files, File{File: f, Size: size})
			return nil
		})
		return files
	}
}
