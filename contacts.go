package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type listContact struct {
	ID               int64
	Email            string
	EmailBlacklisted bool
	ListIDs          []int64
	ListUnsubscribed []int64
	Attributes       map[string]any
}

func (c listContact) onList(id int64) bool {
	return slices.Contains(c.ListIDs, id) || slices.Contains(c.ListUnsubscribed, id)
}

func contactEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email
}

func (c brevoClient) findContact(ctx context.Context, key, email string) (listContact, error) {
	var contact listContact
	err := c.request(ctx, key, http.MethodGet, "contacts/"+url.PathEscape(email)+"?identifierType=email_id", nil, &contact)
	var apiError brevoHTTPError
	if errors.As(err, &apiError) && apiError.status == http.StatusNotFound {
		return listContact{}, nil
	}
	if err == nil && contact.ID <= 0 {
		return contact, fmt.Errorf("Brevo returned a contact without an ID")
	}
	return contact, err
}

type contactAction int

const (
	lookupContact contactAction = iota
	addContact
	removeContact
	unsubscribeContact
)

type contactName struct {
	first, last string
}

func (n contactName) full() string {
	return strings.TrimSpace(strings.TrimSpace(n.first) + " " + strings.TrimSpace(n.last))
}

func (c listContact) nameParts() contactName {
	first, _ := c.Attributes["FIRSTNAME"].(string)
	last, _ := c.Attributes["LASTNAME"].(string)
	return contactName{first: first, last: last}
}

func (c listContact) name() string {
	return c.nameParts().full()
}

func (c brevoClient) changeContact(ctx context.Context, key, email string, listID int64, action contactAction, contact listContact, name contactName) error {
	var payload map[string]any
	switch action {
	case addContact:
		if !contactEmail(email) {
			return fmt.Errorf("enter a valid email address")
		}
		payload = make(map[string]any)
		if !contact.onList(listID) {
			payload["listIds"] = []int64{listID}
		}
		if contact.name() == "" {
			if name.full() == "" {
				return fmt.Errorf("enter a first or last name")
			}
			attributes := make(map[string]string)
			if first := strings.TrimSpace(name.first); first != "" {
				attributes["FIRSTNAME"] = first
			}
			if last := strings.TrimSpace(name.last); last != "" {
				attributes["LASTNAME"] = last
			}
			payload["attributes"] = attributes
		}
		if contact.ID == 0 {
			payload["email"] = email
			return c.request(ctx, key, http.MethodPost, "contacts", payload, nil)
		}
	case removeContact:
		payload = map[string]any{"unlinkListIds": []int64{listID}}
	case unsubscribeContact:
		payload = map[string]any{"emailBlacklisted": true}
	default:
		return fmt.Errorf("unknown contact action")
	}
	if contact.ID <= 0 {
		return fmt.Errorf("look up the contact again")
	}
	endpoint := "contacts/" + strconv.FormatInt(contact.ID, 10) + "?identifierType=contact_id"
	return c.request(ctx, key, http.MethodPut, endpoint, payload, nil)
}

func (c brevoClient) searchListContacts(ctx context.Context, key string, listID int64, query string) ([]listContact, error) {
	contacts, err := c.listContacts(ctx, key, strconv.FormatInt(listID, 10))
	if err != nil {
		return nil, err
	}
	terms := strings.Fields(strings.ToLower(query))
	return slices.DeleteFunc(contacts, func(contact listContact) bool {
		name := strings.ToLower(contact.name())
		return contact.Email == "" || slices.ContainsFunc(terms, func(term string) bool {
			return !strings.Contains(name, term)
		})
	}), nil
}

func (c brevoClient) listContacts(ctx context.Context, key, listID string) ([]listContact, error) {
	const pageSize = 500
	var contacts []listContact
	for offset := 0; ; offset += pageSize {
		var page struct {
			Contacts []listContact
		}
		endpoint := fmt.Sprintf("contacts/lists/%s/contacts?limit=%d&offset=%d", listID, pageSize, offset)
		if err := c.request(ctx, key, http.MethodGet, endpoint, nil, &page); err != nil {
			return nil, err
		}
		contacts = append(contacts, page.Contacts...)
		if len(page.Contacts) < pageSize {
			return contacts, nil
		}
	}
}
