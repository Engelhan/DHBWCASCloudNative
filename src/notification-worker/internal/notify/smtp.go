package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"ticketflow/notification-worker/internal/config"
	"ticketflow/notification-worker/internal/event"
)

// SMTPNotifier sends a real email via SMTP. Only constructed when
// config.Config.SMTPEnabled is true.
type SMTPNotifier struct {
	Host     string
	Port     int
	From     string
	TLSMode  config.TLSMode
	Username string
	Password string
	Timeout  time.Duration
}

func NewSMTPNotifier(cfg config.Config) *SMTPNotifier {
	return &SMTPNotifier{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		From:     cfg.SMTPFrom,
		TLSMode:  cfg.SMTPTLSMode,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		Timeout:  10 * time.Second,
	}
}

func (n *SMTPNotifier) Notify(ctx context.Context, e event.Event) error {
	if e.CustomerEmail == "" {
		return fmt.Errorf("customerEmail is empty, cannot send SMTP notification")
	}

	msg, err := n.buildMessage(e)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, n.Timeout)
	defer cancel()

	// net/smtp has no context-aware API: on timeout we stop waiting here,
	// but the goroutine below keeps running until the underlying TCP dial/
	// I/O itself times out.
	done := make(chan error, 1)
	go func() { done <- n.send(e.CustomerEmail, msg) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (n *SMTPNotifier) send(to string, msg []byte) error {
	addr := net.JoinHostPort(n.Host, strconv.Itoa(n.Port))

	var client *smtp.Client
	var err error

	switch n.TLSMode {
	case config.TLSModeTLS:
		var conn *tls.Conn
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: n.Host})
		if err != nil {
			return fmt.Errorf("tls dial %s: %w", addr, err)
		}
		client, err = smtp.NewClient(conn, n.Host)
	case config.TLSModeStartTLS:
		client, err = smtp.Dial(addr)
		if err == nil {
			if startErr := client.StartTLS(&tls.Config{ServerName: n.Host}); startErr != nil {
				client.Close()
				return fmt.Errorf("starttls: %w", startErr)
			}
		}
	default: // TLSModeNone
		client, err = smtp.Dial(addr)
	}
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer client.Close()

	if n.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", n.Username, n.Password, n.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(n.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close message writer: %w", err)
	}
	return client.Quit()
}

func (n *SMTPNotifier) buildMessage(e event.Event) ([]byte, error) {
	to, err := mail.ParseAddress(e.CustomerEmail)
	if err != nil {
		return nil, fmt.Errorf("invalid customerEmail: %w", err)
	}

	subject, body := renderContent(e)

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", n.From)
	fmt.Fprintf(&b, "To: %s\r\n", to.String())
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String()), nil
}

func renderContent(e event.Event) (subject, body string) {
	eventName := e.EventName
	if eventName == "" {
		eventName = "your event"
	}

	switch e.EventType {
	case event.TypeReservationConfirmed:
		subject = fmt.Sprintf("Reservation confirmed: %s", eventName)
		body = fmt.Sprintf("Your reservation %s for %s has been confirmed.", e.ReservationID, eventName)
		if e.SeatID != "" {
			body += fmt.Sprintf(" Seat: %s.", e.SeatID)
		}
	default: // event.TypeReservationRejected
		subject = fmt.Sprintf("Reservation rejected: %s", eventName)
		body = fmt.Sprintf("Your reservation %s for %s could not be confirmed.", e.ReservationID, eventName)
		if e.Reason != nil && *e.Reason != "" {
			body += fmt.Sprintf(" Reason: %s.", *e.Reason)
		}
	}
	return subject, body
}
