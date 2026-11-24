package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMeetingPacket(t *testing.T) {
	for _, tc := range []struct {
		name string
		test bool
		want []string
	}{
		{"test goes only to the test address", true, []string{"test@example.org"}},
		{"packet excludes opt-outs and duplicates", false, []string{"member@example.org", "second@example.org"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := []packetFile{{Name: "agenda.pdf", Data: []byte("agenda")}, {Name: "minutes.pdf", Data: []byte("minutes")}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/contacts/lists/7/contacts":
					if tc.test {
						t.Error("test send read the club roster")
					}
					fmt.Fprint(w, `{"contacts":[
						{"email":"member@example.org"},
						{"email":"MEMBER@example.org"},
						{"email":"blocked@example.org","emailBlacklisted":true},
						{"email":"unsubscribed@example.org","listUnsubscribed":[7]},
						{"email":"second@example.org","listUnsubscribed":[8]}
					]}`)
				case "/smtp/email":
					var payload struct {
						Attachment      []packetFile
						MessageVersions []packetVersion
						To, CC, BCC     []sender
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					var got, ids []string
					for _, v := range payload.MessageVersions {
						if len(v.To) != 1 {
							t.Errorf("expected one private recipient per message: %+v", v)
							continue
						}
						if v.To[0].Name == "" {
							t.Errorf("recipient %s is missing a display name", v.To[0].Email)
						}
						got = append(got, v.To[0].Email)
						ids = append(ids, v.To[0].Email)
					}
					if !reflect.DeepEqual(got, tc.want) || len(payload.To)+len(payload.CC)+len(payload.BCC) != 0 {
						t.Errorf("unexpected recipients: %v", got)
					}
					if !reflect.DeepEqual(payload.Attachment, files) {
						t.Errorf("unexpected attachments: %+v", payload.Attachment)
					}
					_ = json.NewEncoder(w).Encode(map[string][]string{"messageIds": ids})
				default:
					t.Errorf("unexpected request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			client := brevoClient{base: server.URL + "/", http: server.Client()}
			p := profile{Label: "Club", Footer: "Club", Lists: map[string]string{"7": "Club"}, TestEmail: "test@example.org"}
			d := draft{ListID: "7", Subject: "Agenda", Body: "Meeting agenda"}
			result, err := client.sendPacket(context.Background(), p, d, "<p>Meeting agenda</p>", files, tc.test)
			if err != nil || result.accepted != len(tc.want) || result.total != len(tc.want) || !result.started {
				t.Fatalf("send returned %+v, %v", result, err)
			}
		})
	}
}
