package gwi

import (
	"fmt"
	"io"
	"os"
	"time"
	"strings"
	"path"

	"blmayer.dev/x/gwi/internal/logger"
	"github.com/emersion/go-smtp"
	"github.com/vraycc/go-parsemail"
)

func (g Gwi) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return Session{config: g.config}, nil
}

func (g Gwi) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return Session{}, nil
}

func (g Gwi) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return Session{config: g.config, vault: g.vault, commands: g.commands}, nil
}

// A Session is returned after EHLO.
type Session struct{
	config Config
	vault Vault
	commands map[string]func(from, content, thread string) bool
}

func (s Session) AuthPlain(username, password string) error {
	println("connection sent", username, password)
	return nil
}

func (s Session) Mail(from string, opts smtp.MailOptions) error {
	println("Mail from:", from)
	return nil
}

func (s Session) Rcpt(to string) error {
	println("Rcpt to:", to)
	return nil
}

func (s Session) Data(r io.Reader) error {
	content, err := io.ReadAll(r)
	if err != nil {
		println("read content", err.Error())
		return err
	}

	email, err := parsemail.Parse(strings.NewReader(string(content)))
	if err != nil {
		println("parse email", err.Error())
		return err
	}

	// get user from to field
	from := email.From[0].Address
	userAddress := strings.Split(email.To[0].Address, "@")
	if userAddress[1] != s.config.Domain {
		println("wrong domain:", userAddress[1])
		return fmt.Errorf("wrong domain")
	}
	
	userRepo := strings.Split(userAddress[0], "/")
	user, repo := userRepo[0], userRepo[1]
	if _, err := os.Stat(path.Join(s.config.Root, user, repo)); err != nil {
		println("stat repo", err.Error())
		return err
	}

	// split by subject
	start := strings.Index(email.Subject, "[")
	title := strings.TrimSpace(email.Subject[start:])

	mailDir := path.Join(s.config.Root, user, repo, "mail/open", title)
	err = os.MkdirAll(
		mailDir,
		os.ModeDir|0o700,
	)

	mailFile, err := os.Create(path.Join(mailDir, email.MessageID))
	if err != nil {
		println("create mail file", err.Error())
		return err
	}
	defer mailFile.Close()

	_, err = mailFile.Write(content)
	if err != nil {
		println("write mail file", err.Error())
		return err
	}

	// apply commands
	go func() {
		logger.Debug("applying commands")
		c := string(content)
		for com, f := range s.commands {
			if f(from, c, mailDir) {
				logger.Debug(com, "applied")
			}
		}
	}()

	return err
}

func (s Session) Reset() {}

func (s Session) Logout() error {
	println("logged out")
	return nil
}

func (g *Gwi) NewMailServer() FileMailer {
	s := smtp.NewServer(g)

	s.Addr = g.config.MailAddress
	s.Domain = g.config.Domain
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 2
	s.AllowInsecureAuth = true

	mailer := FileMailer{Root: g.config.Root, Server: s}
	return mailer
}

type FileMailer struct {
	Root string
	*smtp.Server
}

func (g *Gwi) Threads(folder string) ([]Thread, error) {
	logger.Debug("mailer threads for", folder)
	dir, err := os.ReadDir(path.Join(g.config.Root, folder))
	if err != nil {
		logger.Debug("readDir error:", err.Error())
		return nil, err
	}

	var threads []Thread
	for _, d := range dir {
		if !d.IsDir() {
			continue
		}

		info, err := d.Info()
		if err != nil {
			logger.Error("dir info", err.Error())
			continue
		}
		t := Thread{
			Title: d.Name(), 
			LastMod: info.ModTime(),
		}

		threads = append(threads, t)
	}

	return threads, nil
}

func (g *Gwi) Mails(folder string) ([]Email, error) {
	logger.Debug("mailer mails for", folder)

	dir := path.Join(g.config.Root, folder)
	threadDir, err := os.ReadDir(dir)
	if err != nil {
		logger.Error("readDir error:", err.Error())
		return nil, err
	}

	var emails []Email
	for _, t := range threadDir {
		mail, err := g.Mail(path.Join(folder, t.Name()))
		if err != nil {
			logger.Error("mail error:", err.Error())
			continue
		}

		emails = append(emails, mail)
	}
	logger.Debug("found", len(emails), "emails")

	return emails, err
}

func (g *Gwi) Mail(file string) (Email, error) {
	logger.Debug("mailer getting", file)

	mailFile, err := os.Open(path.Join(g.config.Root, file))
	if err != nil {
		logger.Error("open mail error:", err.Error())
		return Email{}, err
	}
	defer mailFile.Close()

	mail, err := parsemail.Parse(mailFile)
	if err != nil {
		logger.Error("email parse error:", err.Error())
		return Email{}, err
	}
	
	email := Email{
		To: mail.To[0].Address,
		From: mail.From[0].Address,
		Date: mail.Date,
		Subject: mail.Subject,
		Body: mail.TextBody,
		Attachments: map[string][]byte{},
	}

	if len(mail.Cc) > 0 {
		email.Cc = mail.Cc[0].Address
	}

	for _, a := range mail.Attachments {
		content, err := io.ReadAll(a.Data)
		if err != nil {
			logger.Error("read attachment", err.Error())
			continue
		}

		email.Attachments[a.Filename] = content
	}
	return email, nil
}

// Close moves a thread to the closed folder
func (g *Gwi) CloseThread(threadPath string) error {
	logger.Debug("closing thread", threadPath)

	// threadPath is like "/.../git/user/repo/mail/open/thread
	thread := path.Base(threadPath)
	dir := path.Dir(path.Dir(threadPath))

	return os.Rename(threadPath, path.Join(dir, "closed", thread))
}
