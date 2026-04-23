package util

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"path/filepath"
	"strings"
	"time"
)

// EmailService sends emails via SMTP with STARTTLS (port 587).
type EmailService struct {
	host        string
	port        string
	username    string
	password    string
	from        string
	fromName    string
	templateDir string // absolute path to the templates/ directory
}

// NewEmailService constructs an EmailService from config values.
// templateDir should be the absolute path to the templates/ directory.
func NewEmailService(host string, port int, username, password, from, fromName, templateDir string) *EmailService {
	return &EmailService{
		host:        host,
		port:        fmt.Sprintf("%d", port),
		username:    username,
		password:    password,
		from:        from,
		fromName:    fromName,
		templateDir: templateDir,
	}
}

// ── Email data structs ────────────────────────────────────────────────────────

type welcomeEmailData struct {
	FirstName string
	Year      int
}

type otpEmailData struct {
	OTP  string
	Year int
}

type passwordResetEmailData struct {
	Email     string
	ChangedAt string
	Year      int
}

// ── Public send methods ───────────────────────────────────────────────────────

// SendWelcomeEmail sends the welcome email to a newly registered parent.
func (s *EmailService) SendWelcomeEmail(toEmail, firstName string) error {
	body, err := s.renderTemplate("headlamp-welcome.html", welcomeEmailData{
		FirstName: firstName,
		Year:      time.Now().Year(),
	})
	if err != nil {
		return fmt.Errorf("welcome email render: %w", err)
	}
	return s.send(toEmail, "Welcome to Headlamp!", body)
}

// SendForgotPasswordEmail sends the forgot-password OTP email.
func (s *EmailService) SendForgotPasswordEmail(toEmail, otp string) error {
	body, err := s.renderTemplate("headlamp-forgot-password.html", otpEmailData{
		OTP:  otp,
		Year: time.Now().Year(),
	})
	if err != nil {
		return fmt.Errorf("forgot-password email render: %w", err)
	}
	return s.send(toEmail, "Your Headlamp Password Reset Code", body)
}

// SendResendOTPEmail sends a fresh OTP via the resend template.
func (s *EmailService) SendResendOTPEmail(toEmail, otp string) error {
	body, err := s.renderTemplate("headlamp-otp-resend.html", otpEmailData{
		OTP:  otp,
		Year: time.Now().Year(),
	})
	if err != nil {
		return fmt.Errorf("resend-otp email render: %w", err)
	}
	return s.send(toEmail, "Your New Headlamp Verification Code", body)
}

// SendPasswordResetEmail sends the password-changed confirmation email.
// changedAt should be a human-readable time string (e.g. "2 Jan 2006, 15:04 UTC").
func (s *EmailService) SendPasswordResetEmail(toEmail, changedAt string) error {
	body, err := s.renderTemplate("headlamp-password-reset.html", passwordResetEmailData{
		Email:     toEmail,
		ChangedAt: changedAt,
		Year:      time.Now().Year(),
	})
	if err != nil {
		return fmt.Errorf("password-reset email render: %w", err)
	}
	return s.send(toEmail, "Your Headlamp Password Has Been Changed", body)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// renderTemplate parses a template file from the templateDir and executes it
// with the given data, returning the rendered HTML string.
func (s *EmailService) renderTemplate(filename string, data any) (string, error) {
	path := filepath.Join(s.templateDir, filename)
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", filename, err)
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", filename, err)
	}
	return buf.String(), nil
}

// send delivers a plain HTML email via STARTTLS.
func (s *EmailService) send(to, subject, htmlBody string) error {
	addr := net.JoinHostPort(s.host, s.port)

	// Connect plain TCP first (STARTTLS upgrades in-band)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer client.Close()

	// Upgrade to TLS via STARTTLS
	tlsConf := &tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
	}
	if err = client.StartTLS(tlsConf); err != nil {
		return fmt.Errorf("smtp starttls: %w", err)
	}

	// Authenticate
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	// Envelope
	fromAddr := fmt.Sprintf("%s <%s>", s.fromName, s.from)
	if err = client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}

	// Message body
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}

	headers := strings.Join([]string{
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		fmt.Sprintf("From: %s", fromAddr),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"",
	}, "\r\n")

	msg := headers + "\r\n" + htmlBody
	if _, err = fmt.Fprint(wc, msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err = wc.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}

	return client.Quit()
}
