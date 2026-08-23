package delivery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSMTPSend(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	messages := make(chan string, 1)
	errors := make(chan error, 1)
	go serveSMTPOnce(listener, messages, errors)

	port := listener.Addr().(*net.TCPAddr).Port
	sender, err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: port, TLS: TLSNone, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewSMTP() error = %v", err)
	}
	err = sender.Send(context.Background(), Message{
		From: "CronPilot <sender@example.com>", To: []string{"owner@example.com"},
		Subject: "CronPilot test", Text: "plain result", HTML: "<b>html result</b>",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	select {
	case serverErr := <-errors:
		t.Fatalf("SMTP server error = %v", serverErr)
	case message := <-messages:
		if !strings.Contains(message, "plain result") || !strings.Contains(message, "html result") || !strings.Contains(message, "multipart/alternative") {
			t.Fatalf("message = %q", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP message")
	}
}

func TestBuildMIMEMessageFoldsLongHTMLLines(t *testing.T) {
	message := Message{
		From:    "CronPilot <sender@example.com>",
		To:      []string{"owner@example.com"},
		Subject: "Verify your email",
		Text:    "Open the verification link.",
		HTML:    "<html><body>" + strings.Repeat("verification content ", 300) + "</body></html>",
	}
	from, recipients, err := validateMessage(message)
	if err != nil {
		t.Fatalf("validateMessage() error = %v", err)
	}
	payload, err := buildMIMEMessage(message, from, recipients)
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}
	for index, line := range bytes.Split(payload, []byte("\r\n")) {
		if len(line) > 998 {
			t.Fatalf("line %d has %d bytes; SMTP permits at most 998", index+1, len(line))
		}
	}

	parsed, err := mail.ReadMessage(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("mail.ReadMessage() error = %v", err)
	}
	mediaType, parameters, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q, %v", mediaType, err)
	}
	parts := multipart.NewReader(parsed.Body, parameters["boundary"])
	foundHTML := false
	for {
		part, nextErr := parts.NextRawPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("NextPart() error = %v", nextErr)
		}
		if !strings.HasPrefix(part.Header.Get("Content-Type"), "text/html") {
			continue
		}
		decoded, readErr := io.ReadAll(quotedprintable.NewReader(part))
		if readErr != nil {
			t.Fatalf("decode HTML part: %v", readErr)
		}
		if string(decoded) != message.HTML {
			t.Fatal("decoded HTML part differs from its source")
		}
		foundHTML = true
	}
	if !foundHTML {
		t.Fatal("HTML MIME part was not found")
	}
}

func serveSMTPOnce(listener net.Listener, messages chan<- string, errors chan<- error) {
	connection, err := listener.Accept()
	if err != nil {
		errors <- err
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	write := func(response string) error {
		if _, writeErr := writer.WriteString(response + "\r\n"); writeErr != nil {
			return writeErr
		}
		return writer.Flush()
	}
	if err := write("220 localhost ESMTP"); err != nil {
		errors <- err
		return
	}
	var message strings.Builder
	inData := false
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			errors <- readErr
			return
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if inData {
			if line == "." {
				inData = false
				messages <- message.String()
				if err := write("250 queued"); err != nil {
					errors <- err
					return
				}
				continue
			}
			message.WriteString(line + "\n")
			continue
		}
		command := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(command, "EHLO"):
			err = write("250 localhost")
		case strings.HasPrefix(command, "MAIL FROM"), strings.HasPrefix(command, "RCPT TO"):
			err = write("250 ok")
		case command == "DATA":
			inData = true
			err = write("354 end with dot")
		case command == "QUIT":
			_ = write("221 bye")
			return
		default:
			err = fmt.Errorf("unexpected SMTP command %s on port %s", line, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
		}
		if err != nil {
			errors <- err
			return
		}
	}
}
