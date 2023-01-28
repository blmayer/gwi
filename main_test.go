package gwi

import (
	"net/http"
	"testing"

	"blmayer.dev/x/gwi/internal/logger"
)

func Test_main(t *testing.T) {
	logger.SetLevel(logger.DebugLevel)

	cfg := Config{
		Domain:      "localhost",
		PagesRoot:   "templates",
		MailAddress: ":2525",
		Root:        "/home/blmayer/repos/gwi/git",
	}

	vault, err := NewFileVault("users.json", "----xxx----")
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(vault.Users)

	g, err := NewFromConfig(cfg, vault)
	if err != nil {
		t.Error(err)
		return
	}

	t.Log("Starting git server at :8080")
	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Fatal(err)
	}
}
