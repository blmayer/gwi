package gwi

import (
	"net/http"
	"strings"
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
		CGIRoot:     "/usr/lib/git-core/git-http-backend",
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

	mailer := g.NewMailServer()
	g.commands = map[string]func(from, content, thread string) bool{
		"close": func(from, content, thread string) bool {
			user := strings.Split(thread, "/")[1]
			if from != vault.GetUser(user).Email() {
				return false
			}

			lineEnd := strings.Index(content, "\n")
			line := strings.TrimSpace(content[:lineEnd])
			if line == "!close" {
				if err := g.CloseThread(thread); err != nil {
					logger.Error("mailer close", err.Error())
					return false
				}
			}
			return true
		},
	}
	t.Run(
		"mail server",
		func(t *testing.T) {
			t.Parallel()
			t.Log("Starting mail server at", cfg.MailAddress)
			if err := mailer.ListenAndServe(); err != nil {
				t.Fatal(err)
			}
		},
	)

	t.Log("Starting git server at :8080")
	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Fatal(err)
	}
}
