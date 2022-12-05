package gwi

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
	"strings"
	"path"

	"github.com/emersion/go-smtp"
	"github.com/vraycc/go-parsemail"
	"blmayer.dev/x/gwi/internal/logger"
)

func (c Config) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return Session{Config: c}, nil
}

func (c Config) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return Session{}, nil
}

func (c Config) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return Session{Config: c}, nil
}

// A Session is returned after EHLO.
type Session struct{
	Config	
}

func (s Session) AuthPlain(username, password string) error {
	logger.Info("connection sent", username, password)
	return nil
}

func (s Session) Mail(from string, opts smtp.MailOptions) error {
	logger.Info("Mail from:", from)
	return nil
}

func (s Session) Rcpt(to string) error {
	logger.Info("Rcpt to:", to)
	return nil
}

func (s Session) Data(r io.Reader) error {
	content, err := io.ReadAll(r)
	if err != nil {
		logger.Error("read content", err.Error())
		return err
	}

	email, err := parsemail.Parse(strings.NewReader(string(content)))
	if err != nil {
		logger.Error("parse email", err.Error())
		return err
	}
	logger.Info("Subject:", email.Subject)

	// get user from to field
	userAddress := strings.Split(email.To[0].Address, "@")
	if userAddress[1] != s.Config.Domain {
		logger.Error("wrong domain:", userAddress[1])
		return fmt.Errorf("wrong domain")
	}
	
	userRepo := strings.Split(userAddress[0], "/")
	user, repo := userRepo[0], userRepo[1]

	if _, err := os.Stat(path.Join(s.Config.Root, user, repo)); err != nil {
		logger.Error("stat repo", err.Error())
		return err
	}

	// split by subject
	start := strings.Index(email.Subject, "[") + 1
	end := strings.Index(email.Subject, "]")
	section := email.Subject[start:end]
	title := strings.TrimSpace(email.Subject[end+1:])

	mailDir := path.Join(s.Config.Root, user, repo, "mail", section, title)
	err = os.Mkdir(
		mailDir,
		os.ModeDir|0o700,
	)
	if err != nil {
		logger.Error("mkdir", err.Error())
		return err
	}

	mailFile, err := os.Create(path.Join(mailDir, email.MessageID))
	if err != nil {
		logger.Error("create mail file", err.Error())
		return err
	}
	defer mailFile.Close()

	mailFile.Write(content)
	return nil
}

func (s Session) Reset() {}

func (s Session) Logout() error {
	logger.Info("logged out")
	return nil
}

func NewMailServer(cfg Config) {
	s := smtp.NewServer(cfg)

	s.Addr = cfg.MailAddress
	s.Domain = "derelict.garden"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 2
	s.AllowInsecureAuth = true

	logger.Info("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
