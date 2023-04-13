package gwi

import (
	"html/template"
	"os"
	"path"
	"sort"
	"syscall"

	"blmayer.dev/x/dovel/interfaces"
	"blmayer.dev/x/gwi/internal/logger"

	git "github.com/libgit2/git2go/v34"
	"github.com/gomarkdown/markdown"
)

type Usage struct {
	Total int
	Free  int
	Used  int
}

type iterator interface {
	Next() (any, error)
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

// func sysInfo() syscall.Sysinfo_t {
// 	sysinfo := syscall.Sysinfo_t{}
// 	err := syscall.Sysinfo(&sysinfo)
// 	if err != nil {
// 		logger.Error("sysinfo:", err)
// 	}
//
// 	sysinfo.Uptime /= 60 * 60
//
// 	mb := uint64(1024 * 1024)
// 	sysinfo.Totalram /= mb
// 	sysinfo.Freeram /= mb
// 	sysinfo.Bufferram /= mb
// 	sysinfo.Sharedram /= mb
// 	sysinfo.Totalswap /= mb
// 	sysinfo.Freeswap /= mb
// 	return sysinfo
// }

func iter(next func() (any, error)) []any {
	var out []any
	for res, err := next(); err == nil; {
		out = append(out, res)
	}
	return out
}

func seq(n uint64) []uint64 {
	out := make([]uint64, n)
	for i := 0; i < int(n); i++ {
		out[i] = uint64(i)
	}
	return out
}

func mdown(in []byte) template.HTML {
	html := markdown.ToHTML(in, nil, nil)
	safeHTML := p.Sanitize(string(html))
	return template.HTML(safeHTML)
}

func (g *Gwi) threads(repo string) func() []interfaces.Mailbox {
	return func() []interfaces.Mailbox {
		logger.Debug("getting threads for", repo)

		mailPath := path.Join(repo, "mail")
		threads, err := g.mailer.Mailboxes(mailPath)
		if err != nil {
			logger.Debug("threads error:", err.Error())
			return nil
		}

		return threads
	}
}

func (g *Gwi) mails(repo string) func(thread string) []interfaces.Email {
	return func(thread string) []interfaces.Email {
		logger.Debug("getting mail for", thread)

		dir := path.Join(repo, "mail", thread)
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

func (g *Gwi) repos(user string) func() []Info {
	return func() []Info {
		logger.Debug("getting repos for", user)
		root := path.Join(g.config.Root, user)
		dir, err := os.ReadDir(root)
		if err != nil {
			logger.Debug("readDir error:", err.Error())
			return nil
		}

		var rs []Info
		for _, d := range dir {
			if !d.IsDir() {
				continue
			}

			info := Info{User: user, Repo: d.Name()}
			info.Git, err = git.OpenRepository(path.Join(root, d.Name()))
			if err != nil {
				logger.Debug("open repo:", err.Error())
				continue
			}
			rs = append(rs, info)
		}

		return rs
	}
}

