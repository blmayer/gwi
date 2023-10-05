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

	g, err := NewFromConfig(cfg, nil)
	if err != nil {
		t.Error(err)
		return
	}

	t.Log("Starting git server at :8080")
	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Fatal(err)
	}
}
