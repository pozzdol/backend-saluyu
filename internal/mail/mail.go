// Package mail sends transactional email through the Resend API, or prints it to the log
// when no API key is configured (development).
package mail

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Message carries both bodies: HTML for mail apps, Text for the rest and for the log.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

//go:embed link.html
var linkHTML string
var linkTmpl = template.Must(template.New("link").Parse(linkHTML))

// Link is an email whose point is one button (verify, reset). Every field is plain text;
// the template escapes it, so a user's name cannot inject markup.
type Link struct {
	Lang, AppURL, Subject, Title, Greeting, Body, Button, URL, Expiry, Fallback, Ignore string
}

// Message renders the link email in the app's look, with a matching plain-text body.
func (l Link) Message() Message {
	var b bytes.Buffer
	if err := linkTmpl.Execute(&b, l); err != nil {
		log.Printf("render link email: %v", err) // the text body still goes out
	}
	text := strings.Join([]string{l.Greeting, l.Body, l.URL, l.Expiry, l.Ignore}, "\n\n")
	return Message{Subject: l.Subject, Text: text, HTML: b.String()}
}

//go:embed digest.html
var digestHTML string
var digestTmpl = template.Must(template.New("digest").Parse(digestHTML))

// Digest is a reminder email: a short list of lines and one button to the app.
type Digest struct {
	Lang, AppURL, Subject, Title, Greeting, Intro, Button, URL, Footer string
	Items                                                              []string
}

func (d Digest) Message() Message {
	var b bytes.Buffer
	if err := digestTmpl.Execute(&b, d); err != nil {
		log.Printf("render digest email: %v", err) // the text body still goes out
	}
	text := d.Greeting + "\n\n" + d.Intro + "\n\n- " + strings.Join(d.Items, "\n- ") + "\n\n" + d.URL + "\n\n" + d.Footer
	return Message{Subject: d.Subject, Text: text, HTML: b.String()}
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
	payload := map[string]any{"from": r.from, "to": []string{m.To}, "subject": m.Subject, "text": m.Text}
	if m.HTML != "" {
		payload["html"] = m.HTML
	}
	body, _ := json.Marshal(payload)
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
