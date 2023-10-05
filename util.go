package gwi

import (
	"os"
	"path"

	"log/slog"
)

func readDesc(repo string) string {
	descBytes, err := os.ReadFile(path.Join(repo, "description"))
	if err != nil {
		slog.Error("read desc", "error", err.Error())
	}
	return string(descBytes)
}
