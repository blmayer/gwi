package gwi

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"

	"blmayer.dev/x/gwi/internal/logger"
	"github.com/EVANA-AG/parsemail"
)

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
			Title:   d.Name(),
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
	fmt.Printf("mail: %+v\n", mail)

	email := Email{
		To:          mail.To[0].Address,
		From:        mail.From[0].Address,
		Date:        mail.Date,
		Subject:     mail.Subject,
		Body:        mail.TextBody,
		Attachments: map[string]Attachment{},
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

		email.Attachments[a.Filename] = Attachment{
			Name:        a.Filename,
			ContentType: a.ContentType,
			Data:        base64.StdEncoding.EncodeToString(content),
		}
	}
	return email, nil
}
