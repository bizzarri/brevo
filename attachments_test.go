package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCampaignAttachment(t *testing.T) {
	const attachment = "https://example.org/agenda.pdf?token=test"
	p := profile{Label: "Club", Footer: "Footer", Lists: map[string]string{"7": "Club"}}
	for _, tc := range []struct {
		name, url, sendPath string
		test                bool
	}{
		{"test with attachment", attachment, "/emailCampaigns/42/sendTest", true},
		{"campaign with attachment", attachment, "/emailCampaigns/42/sendNow", false},
		{"campaign without attachment", "", "/emailCampaigns/42/sendNow", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/emailCampaigns":
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					got, present := payload["attachmentUrl"]
					if present != (tc.url != "") || present && got != tc.url {
						t.Errorf("unexpected attachment: %v (present: %v)", got, present)
					}
					_, _ = io.WriteString(w, `{"id":42}`)
				case tc.sendPath:
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			client := brevoClient{base: server.URL + "/", http: server.Client()}
			d := draft{ListID: "7", Subject: "Agenda", Body: "Meeting agenda", AttachmentURL: tc.url}
			if id, err := client.submit(context.Background(), p, d, "<p>Meeting agenda</p>", tc.test); err != nil || id != 42 {
				t.Fatalf("submit returned %d, %v", id, err)
			}
		})
	}
}
