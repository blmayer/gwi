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
			logger.Error("head file error:", err.Error())
			return ""
		}

		c, err := fileObj.Contents()
		if err != nil {
			logger.Error("file contents error:", err.Error())
			return ""
		}

		return c
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
			files = append(files, File{ File: f, Size: size})
			return nil
		})
		return files
	}
}
