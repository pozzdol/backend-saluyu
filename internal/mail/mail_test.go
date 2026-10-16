package mail

import (
	"strings"
	"testing"
)

func TestLinkEscapesAndKeepsText(t *testing.T) {
	m := Link{Greeting: "Hi <b>Eve</b>,", URL: "http://app.test/verify-email?token=abc", Button: "Verify email"}.Message()
	if strings.Contains(m.HTML, "<b>Eve</b>") || !strings.Contains(m.HTML, "&lt;b&gt;Eve&lt;/b&gt;") {
		t.Fatal("name is not escaped in the HTML body")
	}
	if !strings.Contains(m.HTML, `href="http://app.test/verify-email?token=abc"`) {
		t.Fatal("button link missing from the HTML body")
	}
	if !strings.Contains(m.Text, "http://app.test/verify-email?token=abc\n") {
		t.Fatal("link missing from the text body")
	}
}
