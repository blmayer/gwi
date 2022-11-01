package gwi

import (
	"net/http"
	"testing"

	"blmayer.dev/git/gwi/internal/logger"
)

type testUserStore struct {}

func (t testUserStore) GetByLogin(login string) (User, error) {
	println("getting user from store")
	return User{
		Login: "test",
		Pass: "1234",
	}, nil
}

func Test_main(t *testing.T) {
	logger.Level = logger.DebugLevel

	cfg := Config{
		PagesRoot: "templates",
		Root: "/home/blmayer/repos/gwi/git",
		CGIPrefix: "/",
		Domain: "localhost:8000",
	}

	g, err := NewFromConfig(cfg)
	if err != nil {
		t.Error(err)
		return
	}

	g.SetUserStore(testUserStore{})
	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Error(err)
	}
}
