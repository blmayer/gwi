package gwi

import (
	"encoding/json"
	"fmt"
	"crypto/sha256"
	"os"

	"blmayer.dev/git/gwi/internal/logger"
)

type FileVault struct {
	salt  string
	Users []User
}

func NewFileVault(path, salt string) (FileVault, error) {
	s := FileVault{salt: salt}

	file, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer file.Close()

	s.Users = []User{}
	return s, json.NewDecoder(file).Decode(&s.Users)
}

func (f FileVault) Mix(data string) string {
	bin := sha256.Sum256([]byte(data))
	sum := sha256.Sum256([]byte(f.salt + fmt.Sprintf("%x", bin) + f.salt))
	return fmt.Sprintf("%x", sum)
}

func (f FileVault) Validate(login, pass string) bool {
	logger.Debug("getting login", login)
	for _, u := range f.Users {
		if u.Login == login && u.Pass == f.Mix(pass) {
			return true
		}
	}
	return false
}

