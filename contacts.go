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

func (c brevoClient) changeContact(ctx context.Context, key, email string, listID int64, action contactAction, contact listContact) error {
	if action == addContact && contact.ID == 0 {
		return c.request(ctx, key, http.MethodPost, "contacts", map[string]any{
			"email": email, "listIds": []int64{listID},
		}, nil)
	}
	if contact.ID <= 0 {
		return fmt.Errorf("look up the contact again")
	}
	endpoint := "contacts/" + strconv.FormatInt(contact.ID, 10) + "?identifierType=contact_id"
	var payload map[string]any
	switch action {
	case addContact:
		payload = map[string]any{"listIds": []int64{listID}}
	case removeContact:
		payload = map[string]any{"unlinkListIds": []int64{listID}}
	case unsubscribeContact:
		payload = map[string]any{"emailBlacklisted": true}
	default:
		return fmt.Errorf("unknown contact action")
	}
	return c.request(ctx, key, http.MethodPut, endpoint, payload, nil)
}
