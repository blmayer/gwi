package gwi

import (
	"testing"
)

func Test_Mix(t *testing.T) {
	vault, err := NewFileVault("test.json", "----xxx----")
	if err != nil {
		t.Error(err)
		return
	}

	if res := vault.mix("1234"); res != "759b18c03bf6df73ab38b70a553ccccbd86230f841abba5a6bd4b3b1d7a3937a" {
		t.Error("mix(1234) is ", res)
		return
	}
}
