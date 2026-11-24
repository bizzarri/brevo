package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type sender struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type profile struct {
	Label     string            `json:"label"`
	APIKey    string            `json:"api_key"`
	APIKeyEnv string            `json:"api_key_env"`
	Sender    sender            `json:"sender"`
	ReplyTo   string            `json:"reply_to"`
	TestEmail string            `json:"test_email"`
	Footer    string            `json:"footer"`
	Lists     map[string]string `json:"lists"`
	Templates map[string]string `json:"templates"`
	dir       string
}

var apiKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

func loadProfile(path string) (profile, error) {
	var p profile
	raw, err := os.ReadFile(path)
	if err != nil {
		return p, fmt.Errorf("could not read the profile file: %w", err)
	}
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &p); err != nil {
		return p, fmt.Errorf("check the profile's JSON syntax and field types")
	}
	if p.APIKeyEnv != "" {
		p.APIKey = os.Getenv(p.APIKeyEnv)
	}
	p.APIKey = strings.TrimSpace(p.APIKey)
	if !apiKeyPattern.MatchString(p.APIKey) {
		return p, fmt.Errorf("set the Brevo API key in the profile or its configured environment variable")
	}
	if p.ReplyTo == "" {
		p.ReplyTo = p.Sender.Email
	}
	if strings.TrimSpace(p.Label) == "" || strings.TrimSpace(p.Sender.Name) == "" {
		return p, fmt.Errorf("set the profile label and sender name")
	}
	for _, address := range []string{p.Sender.Email, p.ReplyTo, p.TestEmail} {
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address {
			return p, fmt.Errorf("set a valid sender, reply-to and test email address")
		}
	}
	if len(p.Lists) == 0 {
		return p, fmt.Errorf("configure at least one Brevo list ID and its display name")
	}
	for id, label := range p.Lists {
		if !numericID(id) || strings.TrimSpace(label) == "" {
			return p, fmt.Errorf("each list needs a positive numeric ID and display name")
		}
	}
	if len(p.Templates) == 0 && strings.TrimSpace(p.Footer) == "" {
		return p, fmt.Errorf("configure a template or a footer for plain email")
	}
	for source, label := range p.Templates {
		if (!numericID(source) && !localTemplate(source)) || strings.TrimSpace(label) == "" {
			return p, fmt.Errorf("each template needs a numeric Brevo ID or local .html filename and a display name")
		}
	}
	p.dir = filepath.Dir(path)
	return p, nil
}

func numericID(s string) bool {
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == s
}

func localTemplate(s string) bool {
	return !strings.ContainsAny(s, "/\\:") && strings.EqualFold(filepath.Ext(s), ".html")
}

func sortedIDs(labels map[string]string) []string {
	ids := make([]string, 0, len(labels))
	for id := range labels {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := strings.ToLower(labels[ids[i]]), strings.ToLower(labels[ids[j]])
		if a == b {
			return ids[i] < ids[j]
		}
		return a < b
	})
	return ids
}

type draft struct {
	ListID       string
	Subject      string
	Body         string
	Template     string
	PacketDigest [32]byte
}

func (d draft) validate(p profile) error {
	if _, ok := p.Lists[d.ListID]; !ok {
		return fmt.Errorf("choose a mailing list from this profile")
	}
	if d.Template == "" {
		if strings.TrimSpace(p.Footer) == "" {
			return fmt.Errorf("choose an email template")
		}
	} else if _, ok := p.Templates[d.Template]; !ok {
		return fmt.Errorf("choose an email template from this profile")
	}
	if d.Subject == "" || utf8.RuneCountInString(d.Subject) > 255 || strings.ContainsAny(d.Subject, "\r\n\x00") {
		return fmt.Errorf("enter a subject of up to 255 characters")
	}
	if d.Body == "" || utf8.RuneCountInString(d.Body) > 20000 {
		return fmt.Errorf("enter a message of up to 20,000 characters")
	}

	return nil
}

type campaign struct {
	Name       string             `json:"name"`
	Subject    string             `json:"subject"`
	Sender     sender             `json:"sender"`
	ReplyTo    string             `json:"replyTo"`
	Recipients map[string][]int64 `json:"recipients"`
	HTML       string             `json:"htmlContent"`
}

const bodyMarker = "[Insert body here]"
const maxHTML = 1000000

func inlineHTML(text string) string {
	text = html.EscapeString(text)
	text = strings.NewReplacer("{", "&#123;", "}", "&#125;").Replace(text)
	return strings.ReplaceAll(text, "\n", "<br>")
}

func emailHTML(body, footer string) string {
	var msg strings.Builder
	msg.WriteString(`<!doctype html><html lang="en"><body>`)
	for _, paragraph := range strings.Split(body, "\n\n") {
		msg.WriteString("<p>" + inlineHTML(paragraph) + "</p>")
	}
	msg.WriteString("<hr><p>" + inlineHTML(footer) + "</p>")
	msg.WriteString(`<p><a href="{{ unsubscribe }}">Unsubscribe</a></p></body></html>`)
	return msg.String()
}

func mergeTemplate(template, body string) (string, error) {
	if strings.Count(template, bodyMarker) != 1 {
		return "", fmt.Errorf("the template must contain exactly one %s marker in its HTML", bodyMarker)
	}
	return strings.Replace(template, bodyMarker, inlineHTML(body), 1), nil
}

func (c brevoClient) messageHTML(ctx context.Context, p profile, d draft) (string, error) {
	if d.Template == "" {
		return emailHTML(d.Body, p.Footer), nil
	}
	var template string
	if numericID(d.Template) {
		var result struct {
			HTML string `json:"htmlContent"`
		}
		err := c.request(ctx, p.APIKey, http.MethodGet, "smtp/templates/"+d.Template, nil, &result)
		if err != nil {
			return "", fmt.Errorf("Load Brevo template #%s: %w", d.Template, err)
		}
		template = result.HTML
	} else {
		file, err := os.Open(filepath.Join(p.dir, d.Template))
		if err != nil {
			return "", fmt.Errorf("could not open the local template %s", d.Template)
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, maxHTML+1))
		if err != nil || len(raw) > maxHTML {
			return "", fmt.Errorf("could not read the template or it exceeded 1 MB")
		}
		template = string(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}))
	}
	return mergeTemplate(template, d.Body)
}

type brevoClient struct {
	base string
	http *http.Client
}

func newBrevoClient() brevoClient {
	return brevoClient{
		base: "https://api.brevo.com/v3/",
		http: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type brevoHTTPError struct {
	status  int
	details string
}

func (e brevoHTTPError) Error() string {
	return fmt.Sprintf("Brevo HTTP %d%s", e.status, errorDetails(e.details))
}

func (c brevoClient) request(ctx context.Context, key, method, endpoint string, payload any, result any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode the Brevo request")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+endpoint, body)
	if err != nil {
		return fmt.Errorf("could not create the Brevo request")
	}
	req.Header.Set("api-key", key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("could not confirm a response from Brevo")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var details struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 16<<10)).Decode(&details)
		text := strings.Join(strings.Fields(strings.ReplaceAll(details.Code+" "+details.Message, key, "[redacted]")), " ")
		runes := []rune(text)
		if len(runes) > 380 {
			text = string(runes[:380]) + "…"
		}
		return brevoHTTPError{status: res.StatusCode, details: text}
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(result); err != nil {
			return fmt.Errorf("Brevo returned an unreadable response")
		}
	}
	return nil
}

func errorDetails(text string) string {
	if text == "" {
		return ""
	}
	return ": " + text
}

func (c brevoClient) prepare(ctx context.Context, p profile, d draft) (string, error) {
	if err := d.validate(p); err != nil {
		return "", err
	}
	content, err := c.messageHTML(ctx, p, d)
	if err != nil {
		return "", err
	}
	if len(content) >= maxHTML {
		return "", fmt.Errorf("the finished email HTML must be under 1 MB")
	}
	return content, nil
}

// browserPreview wraps the same campaign HTML in a frame without scripts or forms.
func browserPreview(subject, content string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>` + html.EscapeString(subject) + ` — Brevo Club Mail preview</title>
<style>body{margin:0;display:flex;flex-direction:column;height:100vh;font:18px system-ui}
header{padding:12px 20px;border-bottom:1px solid #ccc;overflow-wrap:anywhere}
iframe{border:0;flex:1;width:100%}</style></head><body>
<header><strong>Subject</strong> ` + html.EscapeString(subject) + `</header><iframe
title="Email design preview" sandbox referrerpolicy="no-referrer"
srcdoc="` + html.EscapeString(content) + `"></iframe></body></html>`
}

func (c brevoClient) submit(ctx context.Context, p profile, d draft, content string, test bool) (int64, error) {
	if err := d.validate(p); err != nil {
		return 0, err
	}
	listID, err := strconv.ParseInt(d.ListID, 10, 64)
	if err != nil || listID <= 0 {
		return 0, fmt.Errorf("invalid Brevo list ID")
	}
	if content == "" || len(content) >= maxHTML {
		return 0, fmt.Errorf("preview the email before sending")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return 0, fmt.Errorf("could not generate a campaign reference")
	}
	mode := " send "
	if test {
		mode = " test "
	}
	payload := campaign{
		Name:    p.Label + mode + hex.EncodeToString(random[:]),
		Subject: d.Subject, Sender: p.Sender, ReplyTo: p.ReplyTo,
		Recipients: map[string][]int64{"listIds": {listID}},
		HTML:       content,
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := c.request(ctx, p.APIKey, http.MethodPost, "emailCampaigns", payload, &created); err != nil {
		return 0, fmt.Errorf("Create campaign: %w", err)
	}
	if created.ID <= 0 {
		return 0, fmt.Errorf("Brevo did not confirm a campaign ID")
	}
	endpoint := "emailCampaigns/" + strconv.FormatInt(created.ID, 10) + "/"
	if test {
		err = c.request(ctx, p.APIKey, http.MethodPost, endpoint+"sendTest", map[string][]string{"emailTo": {p.TestEmail}}, nil)
		if err != nil {
			err = fmt.Errorf("Send test: %w", err)
		}
	} else {
		err = c.request(ctx, p.APIKey, http.MethodPost, endpoint+"sendNow", nil, nil)
		if err != nil {
			err = fmt.Errorf("Send campaign: %w", err)
		}
	}
	return created.ID, err
}
