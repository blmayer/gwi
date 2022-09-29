package gwi

import (
	"net/http"
	"testing"
)

func Test_main(t *testing.T) {
	g, err := NewGWI("templates", "git")
	if err != nil {
		t.Error(err)
		return
	}

	if err := http.ListenAndServe(":8080", g.Handle()); err != nil {
		t.Error(err)
	}
}
