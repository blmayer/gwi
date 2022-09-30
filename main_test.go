package gwi

import (
	"net/http"
	"testing"

	"blmayer.dev/git/gwi/internal/logger"
)

func Test_main(t *testing.T) {
	logger.Level = logger.DebugLevel

	g, err := NewGWI("templates", "/home/blmayer/repos/gwi/git", "/")
	if err != nil {
		t.Error(err)
		return
	}

	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Error(err)
	}
}
