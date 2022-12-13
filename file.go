package gwi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	"blmayer.dev/x/gwi/internal/logger"
)

type vaultUser struct {
	email string
	login string
	pass string
}

type FileVault struct {
	salt  string
	Users map[string]User
}

func (v vaultUser) Email() string {
	return v.email
}

func (v vaultUser) Login() string {
	return v.login
}

func (v vaultUser) Pass() string {
	return v.pass
}

func NewFileVault(path, salt string) (FileVault, error) {
	s := FileVault{salt: salt}

	file, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer file.Close()

	s.Users = map[string]User{}
	users := []vaultUser{}
	err =  json.NewDecoder(file).Decode(&users)
	if err != nil {
		return s, err
	}

	for _, u := range users {
		s.Users[u.login] = u
	}

	return s, nil
}

func (f FileVault) Mix(data string) string {
	bin := sha256.Sum256([]byte(data))
	sum := sha256.Sum256([]byte(f.salt + fmt.Sprintf("%x", bin) + f.salt))
	return fmt.Sprintf("%x", sum)
}

func (f FileVault) GetUser(login string) User {
	return f.Users[login]
}

func (f FileVault) Validate(login, pass string) bool {
	logger.Debug("getting login", login)
	if f.Users[login].Pass() == f.Mix(pass) {
		return true
	}
	return false
}
