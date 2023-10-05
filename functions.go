package gwi

import (
	"html/template"
	"syscall"

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

func mdown(in string) template.HTML {
	html := markdown.ToHTML([]byte(in), nil, nil)
	safeHTML := p.Sanitize(string(html))
	return template.HTML(safeHTML)
}

func wrap(res any, err error) any {
	println(res)
	println(err)
	return res
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
