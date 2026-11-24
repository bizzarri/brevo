package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContactChangesPreserveOtherSubscriptions(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		action                    contactAction
		contact                   listContact
		contactName               contactName
		method, path, query, body string
	}{
		{"create", addContact, listContact{}, contactName{first: " Alice ", last: "Martin"}, "POST", "/contacts", "", `{"attributes":{"FIRSTNAME":"Alice","LASTNAME":"Martin"},"email":"member@example.org","listIds":[7]}`},
		{"add existing", addContact, listContact{ID: 42, EmailBlacklisted: true, Attributes: map[string]any{"FIRSTNAME": "Alice"}}, contactName{}, "PUT", "/contacts/42", "identifierType=contact_id", `{"listIds":[7]}`},
		{"add existing missing name", addContact, listContact{ID: 42, EmailBlacklisted: true}, contactName{last: "Martin"}, "PUT", "/contacts/42", "identifierType=contact_id", `{"attributes":{"LASTNAME":"Martin"},"listIds":[7]}`},
		{"name existing member", addContact, listContact{ID: 42, ListIDs: []int64{7}}, contactName{first: "Alice"}, "PUT", "/contacts/42", "identifierType=contact_id", `{"attributes":{"FIRSTNAME":"Alice"}}`},
		{"remove membership", removeContact, listContact{ID: 42}, contactName{}, "PUT", "/contacts/42", "identifierType=contact_id", `{"unlinkListIds":[7]}`},
		{"unsubscribe and retain lists", unsubscribeContact, listContact{ID: 42}, contactName{}, "PUT", "/contacts/42", "identifierType=contact_id", `{"emailBlacklisted":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.Method != tc.method || r.URL.Path != tc.path || r.URL.RawQuery != tc.query || string(raw) != tc.body {
					t.Errorf("unexpected request: %s %s %s", r.Method, r.URL, raw)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			client := brevoClient{base: server.URL + "/", http: server.Client()}
			if err := client.changeContact(context.Background(), "test-key", "member@example.org", 7, tc.action, tc.contact, tc.contactName); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContactLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("identifierType") != "email_id" {
			t.Errorf("expected email identifier, got %s", r.URL)
		}
		switch r.URL.Path {
		case "/contacts/member+club@example.org":
			_, _ = io.WriteString(w, `{"id":42,"email":"member+club@example.org","listIds":[9],"listUnsubscribed":[7],"emailBlacklisted":true}`)
		case "/contacts/missing@example.org":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	client := brevoClient{base: server.URL + "/", http: server.Client()}
	found, err := client.findContact(context.Background(), "test-key", "member+club@example.org")
	if err != nil || found.ID != 42 || !found.onList(7) || !found.onList(9) || found.onList(8) || !found.EmailBlacklisted {
		t.Fatalf("unexpected lookup: %+v, %v", found, err)
	}
	missing, err := client.findContact(context.Background(), "test-key", "missing@example.org")
	if err != nil || missing.ID != 0 {
		t.Fatalf("expected missing contact, got %+v, %v", missing, err)
	}
	if _, err := client.findContact(context.Background(), "test-key", "forbidden@example.org"); err == nil {
		t.Fatal("expected a permissions error")
	}
}

func TestSearchListContacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contacts/lists/7/contacts" || r.URL.Query().Get("limit") != "500" {
			t.Errorf("unexpected search URL: %s", r.URL)
		}
		var contacts []listContact
		switch r.URL.Query().Get("offset") {
		case "0":
			contacts = make([]listContact, 500)
			contacts[0] = listContact{Email: "other@example.org", Attributes: map[string]any{"FIRSTNAME": "Alice", "LASTNAME": "Jones"}}
		case "500":
			contacts = []listContact{{Email: "match@example.org", EmailBlacklisted: true,
				Attributes: map[string]any{"FIRSTNAME": "Alice", "LASTNAME": "Martin"}}}
		default:
			t.Errorf("unexpected page: %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts})
	}))
	defer server.Close()
	client := brevoClient{base: server.URL + "/", http: server.Client()}
	matches, err := client.searchListContacts(context.Background(), "test-key", 7, " mar  ALI ")
	if err != nil || len(matches) != 1 || matches[0].Email != "match@example.org" || !matches[0].EmailBlacklisted {
		t.Fatalf("unexpected matches: %+v, %v", matches, err)
	}
}

func TestAddContactRequiresNameAndEmail(t *testing.T) {
	client := brevoClient{}
	if err := client.changeContact(context.Background(), "", "", 7, addContact, listContact{}, contactName{first: "Alice"}); err == nil {
		t.Fatal("accepted missing email")
	}
	if err := client.changeContact(context.Background(), "", "member@example.org", 7, addContact, listContact{}, contactName{}); err == nil {
		t.Fatal("accepted missing name")
	}
}
