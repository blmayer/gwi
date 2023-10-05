package gwi

import (
	"log/slog"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *Gwi) file(repo *git.Repository) func(ref plumbing.Hash, name string) string {
	return func(ref plumbing.Hash, name string) string {
		slog.Debug("getting commit", "ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			slog.Error("commit", "error", err.Error())
			return ""
		}

		slog.Debug("getting file", "name", name)
		fileObj, err := commit.File(name)
		if err != nil {
			slog.Error("file", "error", err.Error(), "name", name)
			return ""
		}

		c, err := fileObj.Contents()
		if err != nil {
			slog.Error("contents", "error", err.Error(), "name", name)
			return ""
		}

		return c
	}
}

func (g *Gwi) files(repo *git.Repository) func(ref plumbing.Hash) int {
	return func(ref plumbing.Hash) int {
		// files
		slog.Debug("getting commit", "ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			slog.Error("commit", "error", err.Error())
			return -1
		}

		slog.Debug("getting files", "commit", commit.Hash.String())
		t, err := commit.Tree()
		if err != nil {
			slog.Error("trees", "error", err.Error())
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
				slog.Error("tree tree", "error", err.Error())
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
		slog.Debug("getting commit", "ref", ref.String())
		commit, err := repo.CommitObject(ref)
		if err != nil {
			slog.Error("commit", "error", err.Error())
			return nil
		}

		slog.Debug("getting tree for commit", "hash", commit.Hash.String())
		tree, err := commit.Tree()
		if err != nil {
			slog.Error("trees", "error", err.Error())
			return nil
		}

		var files []File
		tree.Files().ForEach(func(f *object.File) error {
			slog.Debug("getting file", "name", f.Name)
			size, _ := tree.Size(f.Name)
			files = append(files, File{File: f, Size: size})
			return nil
		})
		return files
	}
}
