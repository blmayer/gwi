package gwi

import (
	"net/http"
	"testing"

	"blmayer.dev/x/gwi/internal/logger"
)

func Test_main(t *testing.T) {
	logger.SetLevel(logger.DebugLevel)

	cfg := Config{
		PagesRoot: "templates",
		Root:      "/home/blmayer/gwi/git",
		CGIRoot:   "/usr/lib/git-core/git-http-backend",
	}

	vault, err := NewFileVault("users.json", "----xxx----")
	if err != nil {
		t.Error(err)
		return
	}

	g, err := NewFromConfig(cfg, vault)
	if err != nil {
		t.Error(err)
		return
	}

	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Error(err)
	}
}
