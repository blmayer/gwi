package gwi

import (
	"io"
	"os"
	"path"

	"blmayer.dev/x/gwi/internal/logger"

	"github.com/vraycc/go-parsemail"
)

type FileMailer struct {
	Root string
}

func (m FileMailer) Threads(folder string) ([]Thread, error) {
	logger.Debug("mailer threads for", folder)
	dir, err := os.ReadDir(path.Join(m.Root, folder))
	if err != nil {
		logger.Debug("readDir error:", err.Error())
		return nil, err
	}

	var threads []Thread
	for _, d := range dir {
		if !d.IsDir() {
			continue
		}

		t := Thread{Title: d.Name()}

		threads = append(threads, t)
	}

	return threads, nil
}

func (m FileMailer) Mails(folder string) ([]Email, error) {
	logger.Debug("mailer mails for", folder)

	dir := path.Join(m.Root, folder)
	threadDir, err := os.ReadDir(dir)
	if err != nil {
		logger.Error("readDir error:", err.Error())
		return nil, err
	}

	var emails []Email
	for _, t := range threadDir {
		mail, err := m.Mail(path.Join(folder, t.Name()))
		if err != nil {
			logger.Error("mail error:", err.Error())
			continue
		}

		emails = append(emails, mail)
	}
	logger.Debug("found", len(emails), "emails")

	return emails, err
}

func (m FileMailer) Mail(file string) (Email, error) {
	logger.Debug("mailer getting", file)

	mailFile, err := os.Open(path.Join(m.Root, file))
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
