package gwi

import (
	"os"
	"path"

	"blmayer.dev/git/gwi/internal/logger"
)

func readDesc(repo string) string {
	descBytes, err := os.ReadFile(path.Join(repo, "description"))
	if err != nil {
		logger.Error("read desc error:", err.Error())
	}
	return string(descBytes)
}
