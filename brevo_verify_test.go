package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendAndProfile(t *testing.T) {
	raw, err := os.ReadFile("club-mail.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["footer"] = "Club address"
	decoded["api_key_env"] = "TEST_CLUB_MAIL_KEY"
	t.Setenv("TEST_CLUB_MAIL_KEY", "test-brevo-key-123456")
	raw, _ = json.Marshal(decoded)
	path := filepath.Join(t.TempDir(), "club-mail.json")
	if err := os.WriteFile(path, append([]byte{0xef, 0xbb, 0xbf}, raw...), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := loadProfile(path)
	if err != nil || len(p.Lists) != 2 || len(sortedIDs(p.Lists)) != 2 {
		t.Fatalf("profile %v", err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("api-key") != p.APIKey {
			t.Error("missing API credential")
		}
		switch r.URL.Path {
		case "/emailCampaigns":
			var sent campaign
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			if ids := sent.Recipients["listIds"]; len(ids) != 1 || ids[0] != 201 {
				t.Errorf("wrong list: %v", ids)
			}
			if !strings.Contains(sent.HTML, "&lt;news&gt;") || !strings.Contains(sent.HTML, "{{ unsubscribe }}") {
				t.Error("email formatting")
			}
			fmt.Fprint(w, `{"id":17}`)
		case "/emailCampaigns/17/sendTest":
			var got map[string][]string
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			if emails := got["emailTo"]; len(emails) != 1 || emails[0] != p.TestEmail {
				t.Error("wrong test email")
			}
			w.WriteHeader(204)
		case "/emailCampaigns/17/sendNow":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := newBrevoClient()
	c.base = server.URL + "/"
	if _, err := c.submit(context.Background(), p, draft{ListID: "404", Subject: "Subject", Body: "<news>"}, "<p>invalid</p>", false); err == nil || calls != 0 {
		t.Error("out of profile list reached API")
	}
	for _, test := range []bool{true, false} {
		d := draft{ListID: "201", Subject: "Subject", Body: "<news>"}
		content, err := c.prepare(context.Background(), p, d)
		if err != nil {
			t.Fatal(err)
		}
		id, err := c.submit(context.Background(), p, d, content, test)
		if err != nil || id != 17 {
			t.Fatalf("submission ID %d, error %v", id, err)
		}
	}
	if calls != 4 {
		t.Fatalf("unexpected endpoint count %d", calls)
	}
}

func TestTemplateAndBrevoError(t *testing.T) {
	const footer = `<p>Club address <a href="{{ unsubscribe }}">Unsubscribe</a></p>`
	dir := t.TempDir()
	template := "<html><p><strong>" + bodyMarker + "</strong></p>" + footer + "</html>"
	if err := os.WriteFile(filepath.Join(dir, "newsletter.html"), []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	p := profile{Label: "Club", APIKey: "test-secret-key-1234567", Lists: map[string]string{"201": "List"},
		Templates: map[string]string{"55": "Brevo", "newsletter.html": "Local"}, dir: dir}
	var templateCalls, creates int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/smtp/templates/55":
			templateCalls++
			if r.Method != http.MethodGet {
				t.Error("expected GET")
			}
			json.NewEncoder(w).Encode(map[string]string{"htmlContent": template})
		case "/emailCampaigns":
			creates++
			var sent campaign
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			want := strings.Replace(template, bodyMarker, "&lt;hello&gt;<br>Line two", 1)
			if sent.HTML != want {
				t.Errorf("template output: %q", sent.HTML)
			}
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"code": "invalid_parameter", "message": "Example validation of " + p.APIKey})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := newBrevoClient()
	c.base = server.URL + "/"
	for _, source := range []string{"55", "newsletter.html"} {
		d := draft{ListID: "201", Subject: "Update", Body: "<hello>\nLine two", Template: source}
		content, err := c.prepare(context.Background(), p, d)
		if err != nil {
			t.Fatal(err)
		}
		page := browserPreview(d.Subject, content)
		if !strings.Contains(page, "sandbox") || !strings.Contains(page, `srcdoc="`+html.EscapeString(content)+`"`) {
			t.Error("preview differs from campaign HTML")
		}
		_, err = c.submit(context.Background(), p, d, content, false)
		if err == nil || !strings.Contains(err.Error(), "Create campaign: Brevo HTTP 400") ||
			!strings.Contains(err.Error(), "invalid_parameter Example validation of [redacted]") || strings.Contains(err.Error(), p.APIKey) {
			t.Fatalf("diagnostic: %v", err)
		}
	}
	if templateCalls != 1 || creates != 2 {
		t.Fatalf("template GETs=%d, creates=%d", templateCalls, creates)
	}
	for _, content := range []string{"missing", bodyMarker + bodyMarker} {
		if _, err := mergeTemplate(content, "hello"); err == nil {
			t.Error("invalid marker count accepted")
		}
	}
}
