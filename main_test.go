package gwi

import (
	"net/http"
	"testing"

	"blmayer.dev/git/gwi/internal/logger"
)

func Test_main(t *testing.T) {
	logger.Level = logger.DebugLevel

	cfg := Config{
		PagesRoot: "templates",
		Root: "/home/blmayer/gwi/git",
		CGIPrefix: "/",
		CGIRoot: "/usr/lib/git-core/git-http-backend",
		Domain: "localhost:8000",
	}

	userStore, err := NewFileStorage("users.json")
	if err != nil {
		t.Error(err)
		return
	}

	g, err := NewFromConfig(cfg, userStore)
	if err != nil {
		t.Error(err)
		return
	}
	logger.Debug("g:", g)
	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Error(err)
	}
}
