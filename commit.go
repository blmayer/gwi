package gwi

import (
	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *Gwi) commits(repo *git.Repository) func(ref plumbing.Hash) []*object.Commit {
	return func(ref plumbing.Hash) []*object.Commit {
		logger.Debug("getting log for ref", ref.String())
		logs, err := repo.Log(&git.LogOptions{From: ref})
		if err != nil {
			logger.Error("commits", err.Error())
			return nil
		}

		commits := make([]*object.Commit, 0, 200)
		for count := 200; count > 0; count-- {
			c, _ := logs.Next()

			if c == nil {
				break
			}
			commits = append(commits, c)
		}
		return commits
	}
}

func (g *Gwi) commit(repo *git.Repository) func(ref plumbing.Hash) *object.Commit {
	return func(ref plumbing.Hash) *object.Commit {
		logger.Debug("getting commit", ref.String())
		c, err := repo.CommitObject(ref)
		if err != nil {
			logger.Error("commit", err.Error())
		}

		return c
	}
}
