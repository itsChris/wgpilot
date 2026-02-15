package notify

import (
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPConfig holds the SMTP server configuration.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	TLS      bool
}

// SMTPNotifier sends email notifications via SMTP.
type SMTPNotifier struct {
	cfg SMTPConfig
}

// NewSMTPNotifier creates an SMTPNotifier with the given config.
func NewSMTPNotifier(cfg SMTPConfig) (*SMTPNotifier, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp: host is required")
	}
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("smtp: from address is required")
	}
	return &SMTPNotifier{cfg: cfg}, nil
}

// Send sends an email to the given recipients.
func (n *SMTPNotifier) Send(to []string, subject, body string) error {
	if len(to) == 0 {
		return fmt.Errorf("smtp: no recipients specified")
	}

	addr := fmt.Sprintf("%s:%s", n.cfg.Host, n.cfg.Port)

	msg := buildMessage(n.cfg.From, to, subject, body)

	var auth smtp.Auth
	if n.cfg.Username != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, n.cfg.From, to, []byte(msg)); err != nil {
		return fmt.Errorf("smtp: send mail: %w", err)
	}

	return nil
}

func buildMessage(from string, to []string, subject, body string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
