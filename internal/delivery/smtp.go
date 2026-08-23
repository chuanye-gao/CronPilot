package delivery

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/id"
)

const (
	TLSImplicit = "implicit"
	TLSStartTLS = "starttls"
	TLSNone     = "none"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      string
	Timeout  time.Duration
}

type SMTP struct {
	config SMTPConfig
}

func NewSMTP(config SMTPConfig) (*SMTP, error) {
	config.Host = strings.TrimSpace(config.Host)
	config.TLS = strings.ToLower(strings.TrimSpace(config.TLS))
	if config.Host == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("smtp port must be between 1 and 65535")
	}
	if config.TLS != TLSImplicit && config.TLS != TLSStartTLS && config.TLS != TLSNone {
		return nil, fmt.Errorf("smtp tls must be implicit, starttls, or none")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, fmt.Errorf("smtp username and password must be configured together")
	}
	if config.Timeout <= 0 {
		config.Timeout = 20 * time.Second
	}
	return &SMTP{config: config}, nil
}

func (s *SMTP) Send(ctx context.Context, message Message) error {
	from, recipients, err := validateMessage(message)
	if err != nil {
		return err
	}
	payload, err := buildMIMEMessage(message, from, recipients)
	if err != nil {
		return err
	}

	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	dialer := &net.Dialer{Timeout: s.config.Timeout}
	tlsConfig := &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
	var connection net.Conn
	if s.config.TLS == TLSImplicit {
		connection, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("connect smtp server: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(s.deadline(ctx)); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(connection, s.config.Host)
	if err != nil {
		return fmt.Errorf("start smtp client: %w", err)
	}
	defer client.Close()
	if s.config.TLS == TLSStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("start smtp tls: %w", err)
		}
	}
	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticate smtp client: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("set smtp sender: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient.Address); err != nil {
			return fmt.Errorf("set smtp recipient %q: %w", recipient.Address, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start smtp message: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish smtp message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit smtp client: %w", err)
	}
	return nil
}

func (s *SMTP) deadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(s.config.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

func validateMessage(message Message) (*mail.Address, []*mail.Address, error) {
	from, err := mail.ParseAddress(message.From)
	if err != nil {
		return nil, nil, fmt.Errorf("parse email sender: %w", err)
	}
	if len(message.To) == 0 {
		return nil, nil, fmt.Errorf("email requires at least one recipient")
	}
	recipients := make([]*mail.Address, 0, len(message.To))
	for _, value := range message.To {
		recipient, parseErr := mail.ParseAddress(value)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("parse email recipient %q: %w", value, parseErr)
		}
		recipients = append(recipients, recipient)
	}
	if strings.ContainsAny(message.Subject, "\r\n") {
		return nil, nil, fmt.Errorf("email subject contains a line break")
	}
	return from, recipients, nil
}

func buildMIMEMessage(message Message, from *mail.Address, recipients []*mail.Address) ([]byte, error) {
	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	messageDomain := "cronpilot.local"
	if parts := strings.SplitN(from.Address, "@", 2); len(parts) == 2 && parts[1] != "" {
		messageDomain = parts[1]
	}
	headers := []string{
		"From: " + from.String(),
		"To: " + joinAddresses(recipients),
		"Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"Message-ID: <" + id.New("mail") + "@" + messageDomain + ">",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=" + strconv.Quote(multipartWriter.Boundary()),
		"",
	}
	for _, header := range headers {
		body.WriteString(header + "\r\n")
	}
	if err := writeQuotedPrintablePart(multipartWriter, "text/plain", message.Text); err != nil {
		return nil, fmt.Errorf("write text email part: %w", err)
	}
	if err := writeQuotedPrintablePart(multipartWriter, "text/html", message.HTML); err != nil {
		return nil, fmt.Errorf("write html email part: %w", err)
	}
	if err := multipartWriter.Close(); err != nil {
		return nil, fmt.Errorf("finish mime message: %w", err)
	}
	return body.Bytes(), nil
}

func writeQuotedPrintablePart(writer *multipart.Writer, mediaType, content string) error {
	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Type", mediaType+`; charset="UTF-8"`)
	headers.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := writer.CreatePart(headers)
	if err != nil {
		return err
	}
	encoded := quotedprintable.NewWriter(part)
	if _, err := encoded.Write([]byte(content)); err != nil {
		_ = encoded.Close()
		return err
	}
	return encoded.Close()
}

func joinAddresses(addresses []*mail.Address) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, address.String())
	}
	return strings.Join(values, ", ")
}
