package gwi

import (
	"encoding/json"
	"os"

	"blmayer.dev/git/gwi/internal/logger"
)

type FileStorage struct {
	Users []User
}

func NewFileStorage(path string) (FileStorage, error) {
	s := FileStorage{}

	file, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer file.Close()

	s.Users = []User{}
	return s, json.NewDecoder(file).Decode(&s.Users)
}

func (f FileStorage) GetByLogin(login string) (User, error) {
	logger.Debug("getting login", login)
	for _, u := range f.Users {
		if u.Login == login {
			return u, nil
		}
	}
	return User{}, nil
}
