package gwi

import (
	"html/template"
	"os"
	"path"
	"sort"
	"syscall"

	"blmayer.dev/x/dovel/interfaces"
	"blmayer.dev/x/gwi/internal/logger"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gomarkdown/markdown"
)

type Usage struct {
	Total int
	Free  int
	Used  int
}

// diskUsage returns total and used Mb
func diskUsage() Usage {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs("/", &fs)
	if err != nil {
		return Usage{}
	}

	mb := uint64(1024 * 1024)
	return Usage{
		Total: int(fs.Blocks * uint64(fs.Bsize) / mb),
		Free:  int(fs.Bfree * uint64(fs.Bsize) / mb),
		Used:  int((fs.Blocks - fs.Bfree) * uint64(fs.Bsize) / mb),
	}
}

func sysInfo() syscall.Sysinfo_t {
	sysinfo := syscall.Sysinfo_t{}
	err := syscall.Sysinfo(&sysinfo)
	if err != nil {
		logger.Error("sysinfo:", err)
	}

	sysinfo.Uptime /= 60 * 60

	mb := uint64(1024 * 1024)
	sysinfo.Totalram /= mb
	sysinfo.Freeram /= mb
	sysinfo.Bufferram /= mb
	sysinfo.Sharedram /= mb
	sysinfo.Totalswap /= mb
	sysinfo.Freeswap /= mb
	return sysinfo
}

func mdown(in string) template.HTML {
	html := markdown.ToHTML([]byte(in), nil, nil)
	safeHTML := p.Sanitize(string(html))
	return template.HTML(safeHTML)
}

func (g *Gwi) thread(user, repo string) func(section string) []interfaces.Mailbox {
	return func(section string) []interfaces.Mailbox {
		logger.Debug("getting threads for", section)

		mailPath := path.Join(user, repo, "mail", section)
		threads, err := g.mailer.Mailboxes(mailPath)
		if err != nil {
			logger.Debug("threads error:", err.Error())
			return nil
		}

		return threads
	}
}

func (g *Gwi) mails(user, repo string) func(thread string) []interfaces.Email {
	return func(thread string) []interfaces.Email {
		logger.Debug("getting mail for", thread)

		dir := path.Join(user, repo, "mail", thread)
		mail, err := g.mailer.Mails(dir)
		if err != nil {
			logger.Error("read mail", err.Error())
			return nil
		}

		sort.Slice(
			mail,
			func(i, j int) bool {
				return mail[i].Date.Before(mail[j].Date)
			},
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
