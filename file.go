package gwi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

type vaultUser struct {
	Address  string
	Name     string
	Password string
}

// FileVault is one example implementation that reads users from a JSON
// file. It implements the [Vault] interface, which is used to authorize/
// authenticate users.
type FileVault struct {
	salt  string
	Users map[string]User
}

func (v vaultUser) Email() string {
	return v.Address
}

func (v vaultUser) Login() string {
	return v.Name
}

func (v vaultUser) Pass() string {
	return v.Password
}

// NewFileVault creates a Vault that uses the file at path as user database.
// The salt parameter is used to fuzz user's passwords.
func NewFileVault(path, salt string) (FileVault, error) {
	s := FileVault{salt: salt, Users: map[string]User{}}

	file, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer file.Close()

	s.Users = map[string]User{}
	users := []vaultUser{}
	err = json.NewDecoder(file).Decode(&users)
	if err != nil {
		return s, err
	}

	for _, u := range users {
		s.Users[u.Name] = u
	}

	return s, nil
}

func (f FileVault) mix(data string) string {
	bin := sha256.Sum256([]byte(data))
	sum := sha256.Sum256([]byte(f.salt + fmt.Sprintf("%x", bin) + f.salt))
	return fmt.Sprintf("%x", sum)
}

func (f FileVault) GetUser(login string) User {
	return f.Users[login]
}

// Validate is used to check if a user and pass combination is valid. This is
// used on git receive pack. User and pass parameters are received from HTTP
// Basic authorization flow.
func (f FileVault) Validate(login, pass string) bool {
	slog.Debug("getting login", "login", login)
	if f.Users[login].Pass() == f.mix(pass) {
		return true
	}
	return false
}
