package main

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	wmContactResult = 0x8003
	switchScreenID  = 20 + iota
	contactListID
	contactEmailID
	findContactID
	addContactID
	removeContactID
	unsubscribeContactID
	searchContactsID
	contactMatchesID
)

type contactsUI struct {
	switchScreen, contactListLabel, contactLists, emailLabel, emailField uintptr
	find, addMember, removeMember, unsubscribe, contactDetails           uintptr
	search, matchesList                                                  uintptr
	matches                                                              []listContact
	choosingMatch                                                        bool
	managing, contactChecked                                             bool
	selectedContact                                                      listContact
	contactDone                                                          chan contactResult
}

type contactResult struct {
	contact    listContact
	action     contactAction
	err        error
	refreshErr error
	searching  bool
	matches    []listContact
}

func (a *application) initContacts() {
	a.contactDone = make(chan contactResult, 1)
	a.switchScreen = a.control("BUTTON", "Manage mailing lists", wsTab, 0, switchScreenID)
	a.contactListLabel = a.control("STATIC", "Mailing list", 0, 0, 0)
	a.contactLists = a.control("COMBOBOX", "", wsTab|0x00200003, 0, contactListID)
	a.emailLabel = a.control("STATIC", "Contact name or email address", 0, 0, 0)
	a.emailField = a.control("EDIT", "", wsTab|0x80, 0x200, contactEmailID)
	send.Call(a.emailField, 0xc5, 254, 0)
	a.search = a.control("BUTTON", "Search names", wsTab, 0, searchContactsID)
	a.matchesList = a.control("LISTBOX", "Matching contacts", wsTab|0x00200101, 0x200, contactMatchesID)
	a.find = a.control("BUTTON", "Look up", wsTab, 0, findContactID)
	a.addMember = a.control("BUTTON", "Add to list", wsTab, 0, addContactID)
	a.removeMember = a.control("BUTTON", "Remove from list", wsTab, 0, removeContactID)
	a.unsubscribe = a.control("BUTTON", "Unsubscribe email…", wsTab, 0, unsubscribeContactID)
	a.contactDetails = a.control("EDIT", "", wsTab|0x00200844, 0x200, 0)
}

func (a *application) layoutContacts(set func(uintptr, int, int, int, int), width, height int) {
	set(a.switchScreen, 16, 92, 280, 40)
	set(a.contactListLabel, 16, 140, width-32, 35)
	set(a.contactLists, 16, 176, width-32, 250)
	set(a.emailLabel, 16, 221, width-32, 35)
	set(a.emailField, 16, 257, width-350, 40)
	set(a.find, width-322, 257, 134, 40)
	set(a.search, width-178, 257, 162, 40)
	buttonWidth := (width - 56) / 3
	set(a.addMember, 16, 310, buttonWidth, 42)
	set(a.removeMember, 28+buttonWidth, 310, buttonWidth, 42)
	set(a.unsubscribe, 40+2*buttonWidth, 310, buttonWidth, 42)
	detailsWidth := width - 32
	if len(a.matches) > 0 {
		detailsWidth = (width - 48) / 2
		set(a.matchesList, 32+detailsWidth, 365, detailsWidth, height-381)
	}
	set(a.contactDetails, 16, 365, detailsWidth, height-381)
}

func (a *application) updateContacts() {
	var visible uintptr
	if a.managing {
		visible = 5
	}
	for _, control := range []uintptr{a.contactListLabel, a.contactLists, a.emailLabel,
		a.emailField, a.find, a.search, a.addMember, a.removeMember, a.unsubscribe, a.contactDetails} {
		show.Call(control, visible)
	}
	show.Call(a.matchesList, 0)
	if a.managing && len(a.matches) > 0 {
		show.Call(a.matchesList, 5)
	}
	ready := a.managing && a.path != "" && !a.busy
	listID, _ := a.contactList()
	onList := a.selectedContact.onList(listID)
	for _, control := range []struct {
		handle uintptr
		active bool
	}{
		{a.switchScreen, a.review == nil && !a.busy},
		{a.contactLists, ready}, {a.emailField, ready},
		{a.matchesList, ready},
		{a.search, ready && listID > 0 && strings.TrimSpace(value(a.emailField)) != ""},
		{a.find, ready && listID > 0 && contactEmail(strings.TrimSpace(value(a.emailField)))},
		{a.addMember, ready && a.contactChecked && !onList},
		{a.removeMember, ready && a.contactChecked && onList},
		{a.unsubscribe, ready && a.contactChecked && onList && !a.selectedContact.EmailBlacklisted},
	} {
		var active uintptr
		if control.active {
			active = 1
		}
		enable.Call(control.handle, active)
	}
}

func (a *application) contactList() (int64, string) {
	index := comboIndex(a.contactLists)
	if index < 0 || index >= len(a.ids) {
		return 0, ""
	}
	id := a.ids[index]
	number, _ := strconv.ParseInt(id, 10, 64)
	return number, a.profile.Lists[id]
}

func (a *application) resetContactLists() {
	send.Call(a.contactLists, cbReset, 0, 0)
	for _, id := range a.ids {
		add(a.contactLists, a.profile.Lists[id]+" (#"+id+")")
	}
	if len(a.ids) == 1 {
		send.Call(a.contactLists, cbSet, 0, 0)
	}
	caption(a.emailField, "")
	a.clearMatches()
	a.clearContact()
}

func (a *application) clearMatches() {
	a.matches = nil
	send.Call(a.matchesList, 0x184, 0, 0) // LB_RESETCONTENT
	a.layout()
}

func (a *application) clearContact() {
	a.contactChecked = false
	a.selectedContact = listContact{}
	prompt := "Choose a mailing list. Search by name or look up an exact email address."
	if a.path == "" {
		prompt = "Select a valid account profile. See Compose email for profile status."
	}
	caption(a.contactDetails, prompt)
}

func (a *application) contactCommand(id, notification uintptr) bool {
	switch {
	case id == switchScreenID && !a.busy && a.review == nil:
		a.managing = !a.managing
		label := "Manage mailing lists"
		if a.managing {
			label = "Compose email"
		}
		caption(a.switchScreen, label)
		a.update()
	case id == contactListID && notification == 1 || id == contactEmailID && notification == 0x300:
		if !a.busy && !a.choosingMatch {
			a.clearMatches()
			a.clearContact()
			a.updateContacts()
		}
	case id == searchContactsID && notification == 0:
		a.searchContacts()
	case id == contactMatchesID && notification == 1 && !a.busy:
		index, _, _ := send.Call(a.matchesList, 0x188, 0, 0) // LB_GETCURSEL
		if index < uintptr(len(a.matches)) {
			a.choosingMatch = true
			caption(a.emailField, a.matches[index].Email)
			a.choosingMatch = false
			a.runContact(lookupContact)
		}
	case id >= findContactID && id <= unsubscribeContactID && notification == 0:
		a.runContact(contactAction(id - findContactID))
	default:
		return false
	}
	return true
}

func (a *application) runContact(action contactAction) {
	if !a.managing || a.busy || a.path == "" {
		return
	}
	listID, label := a.contactList()
	email := strings.TrimSpace(value(a.emailField))
	if listID <= 0 || !contactEmail(email) {
		return
	}
	selected := a.selectedContact
	onList := selected.onList(listID)
	if action != lookupContact && !a.contactChecked {
		return
	}
	var confirmation string
	switch action {
	case addContact:
		if onList {
			return
		}
		confirmation = fmt.Sprintf("Add %s to %s? Confirm that the contact has agreed to receive mail from this list.", email, label)
	case removeContact:
		if !onList {
			return
		}
		confirmation = fmt.Sprintf("Remove %s from %s? Their Brevo contact record and other list memberships will be retained.", email, label)
	case unsubscribeContact:
		if !onList || selected.EmailBlacklisted {
			return
		}
		confirmation = fmt.Sprintf("Unsubscribe %s from email campaigns across this Brevo account? The contact will stay visible on their mailing lists.", email)
	}
	if confirmation != "" && popup(a.window, "Confirm mailing list change", confirmation, 0x124) != 6 {
		return
	}
	key, window := a.profile.APIKey, a.window
	a.busy = true
	a.contactChecked = false
	caption(a.contactDetails, "Contacting Brevo…")
	a.update()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		client := newBrevoClient()
		result := contactResult{action: action}
		if action != lookupContact {
			result.err = client.changeContact(ctx, key, email, listID, action, selected)
		}
		if result.err == nil {
			result.contact, result.refreshErr = client.findContact(ctx, key, email)
		}
		a.contactDone <- result
		post.Call(window, wmContactResult, 0, 0)
	}()
}

func (a *application) finishContact(result contactResult) {
	a.busy = false
	defer a.update()
	if result.searching {
		a.showMatches(result)
		return
	}
	if result.err != nil {
		caption(a.contactDetails, "Change could not be confirmed. Look up the contact again before retrying.\r\n"+result.err.Error())
		return
	}
	if result.refreshErr != nil {
		message := "Lookup failed. "
		if result.action != lookupContact {
			message = "Brevo accepted the change. Look up the contact again to refresh its status.\r\n"
		}
		caption(a.contactDetails, message+result.refreshErr.Error())
		return
	}
	contact := result.contact
	a.selectedContact, a.contactChecked = contact, true
	if contact.ID == 0 {
		caption(a.contactDetails, "No contact found for this email address. Add to list will create the contact.")
		return
	}
	listID, label := a.contactList()
	membership := "Outside this list"
	if contact.onList(listID) {
		membership = "On this list"
	}
	emailStatus := "Email campaigns allowed"
	if contact.EmailBlacklisted {
		emailStatus = "Email campaigns blocked across this Brevo account"
	} else {
		for _, id := range contact.ListUnsubscribed {
			if id == listID {
				emailStatus = "Unsubscribed from this list"
				break
			}
		}
	}
	caption(a.contactDetails, fmt.Sprintf("%s\r\n%s\r\n%s: %s\r\n%s",
		contact.Email, contact.name(), label, membership, emailStatus))
}

func (a *application) searchContacts() {
	if !a.managing || a.busy || a.path == "" {
		return
	}
	listID, _ := a.contactList()
	query := strings.TrimSpace(value(a.emailField))
	if listID <= 0 || query == "" {
		return
	}
	key, window, done := a.profile.APIKey, a.window, a.contactDone
	a.clearMatches()
	a.clearContact()
	a.busy = true
	caption(a.contactDetails, "Searching the selected list…")
	a.update()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		matches, err := newBrevoClient().searchListContacts(ctx, key, listID, query)
		done <- contactResult{searching: true, matches: matches, err: err}
		post.Call(window, wmContactResult, 0, 0)
	}()
}

func (a *application) showMatches(result contactResult) {
	if result.err != nil {
		caption(a.contactDetails, "Search failed. "+result.err.Error())
		return
	}
	a.matches = result.matches
	if len(a.matches) == 0 {
		caption(a.contactDetails, "No matching names on this list.")
		return
	}
	for _, contact := range a.matches {
		label := wide(contact.name() + " (" + contact.Email + ")")
		send.Call(a.matchesList, 0x180, 0, ptr(label)) // LB_ADDSTRING
		runtime.KeepAlive(label)
	}
	a.layout()
	caption(a.contactDetails, fmt.Sprintf("Matches: %d. Select a contact to review their details.", len(a.matches)))
}
