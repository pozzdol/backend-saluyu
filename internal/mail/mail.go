// Package mail sends transactional email through the Resend API, or prints it to the log
// when no API key is configured (development).
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

// New picks Resend when apiKey is set, the log otherwise.
func New(apiKey, from string) Sender {
	if apiKey == "" {
		return Log{}
	}
	return &Resend{key: apiKey, from: from, client: &http.Client{Timeout: 10 * time.Second}}
}

// Log prints the email instead of sending it, so reset links work in development.
type Log struct{}

func (Log) Send(_ context.Context, m Message) error {
	log.Printf("mail (not sent, RESEND_API_KEY empty) to=%s subject=%q\n%s", m.To, m.Subject, m.Text)
	return nil
}

type Resend struct {
	key, from string
	client    *http.Client
}

func (r *Resend) Send(ctx context.Context, m Message) error {
	body, _ := json.Marshal(map[string]any{"from": r.from, "to": []string{m.To}, "subject": m.Subject, "text": m.Text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("resend: %s: %s", res.Status, msg)
	}
	return nil
}
