package service

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"slices"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/model"
)

// Unlike the reference (which reads package-level globals populated from the
// option map), this implementation reads SMTP settings on demand from the
// option store via model.GetOption*. This keeps it in the service layer, since
// the common package cannot import model.

// EmailLoginAuthServerList holds SMTP servers that require AUTH LOGIN instead
// of AUTH PLAIN. Standard providers (e.g. Brevo) authenticate with PLAIN.
var EmailLoginAuthServerList = []string{
	"smtp.sendcloud.net",
	"smtp.azurecomm.net",
}

// SendEmail sends an HTML email to receiver. Multiple recipients may be
// separated by ";". Connection details are read from the option store.
func SendEmail(subject string, receiver string, content string) error {
	server := model.GetOptionString("SMTPServer")
	port := model.GetOptionInt("SMTPPort")
	account := model.GetOptionString("SMTPAccount")
	token := model.GetOptionString("SMTPToken")
	from := model.GetOptionString("SMTPFrom")
	sslEnabled := model.GetOptionBool("SMTPSSLEnabled")
	systemName := model.GetOptionString("SystemName")

	if from == "" { // for compatibility
		from = account
	}
	id, err2 := generateMessageID(from)
	if err2 != nil {
		return err2
	}
	if server == "" && account == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}

	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s <%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, systemName, from, encodedSubject, time.Now().Format(time.RFC1123Z), id, content))

	auth := getSMTPAuth(account, token, server)
	addr := fmt.Sprintf("%s:%d", server, port)
	to := strings.Split(receiver, ";")

	var err error
	if port == 465 || sslEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         server,
		}
		conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", server, port), tlsConfig)
		if err != nil {
			return err
		}
		client, err := smtp.NewClient(conn, server)
		if err != nil {
			return err
		}
		defer client.Close()
		if err = client.Auth(auth); err != nil {
			return err
		}
		if err = client.Mail(from); err != nil {
			return err
		}
		receiverEmails := strings.Split(receiver, ";")
		for _, r := range receiverEmails {
			if err = client.Rcpt(r); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(mail)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}
	} else {
		err = smtp.SendMail(addr, auth, from, to, mail)
	}
	if err != nil {
		log.Printf("failed to send email to %s: %v", receiver, err)
	}
	return err
}

func generateMessageID(from string) (string, error) {
	split := strings.Split(from, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := split[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), randomString(12), domain), nil
}

// randomString returns n hex characters of cryptographic randomness.
func randomString(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func shouldUseSMTPLoginAuth(server, account string) bool {
	if model.GetOptionBool("SMTPForceAuthLogin") {
		return true
	}
	return isOutlookServer(account) || slices.Contains(EmailLoginAuthServerList, server)
}

func getSMTPAuth(account, token, server string) smtp.Auth {
	if shouldUseSMTPLoginAuth(server, account) {
		return LoginAuth(account, token)
	}
	return smtp.PlainAuth("", account, token, server)
}

func isOutlookServer(account string) bool {
	return strings.Contains(account, "outlook") || strings.Contains(account, "onmicrosoft")
}

// loginAuth implements smtp.Auth using the AUTH LOGIN mechanism, required by
// some providers (e.g. Outlook).
type loginAuth struct {
	username string
	password string
}

// LoginAuth returns an smtp.Auth that authenticates via AUTH LOGIN.
func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown fromServer")
		}
	}
	return nil, nil
}
