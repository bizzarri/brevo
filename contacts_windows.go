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
	contactFirstNameID
	contactLastNameID
	listMembersID
	previousMembersID
	nextMembersID
)

type contactsUI struct {
	listMembers, previousMembers, nextMembers, memberRange               uintptr
	browsingMembers, hasNextMembers                                      bool
	memberOffset                                                         int
	firstLabel, firstName, lastLabel, lastName                           uintptr
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
	memberPage, hasNextMembers bool
	memberOffset               int
	contact                    listContact
	action                     contactAction
	err                        error
	refreshErr                 error
	searching                  bool
	matches                    []listContact
}

func (a *application) initContacts() {
	a.contactDone = make(chan contactResult, 1)
	if listManagerMode != "1" {
		a.switchScreen = a.control("BUTTON", "Manage mailing lists", wsTab, 0, switchScreenID)
	}
	a.contactListLabel = a.control("STATIC", "Mailing list", 0, 0, 0)
	a.contactLists = a.control("COMBOBOX", "", wsTab|0x00200003, 0, contactListID)
	a.emailLabel = a.control("STATIC", "Contact name or email address", 0, 0, 0)
	a.emailField = a.control("EDIT", "", wsTab|0x80, 0x200, contactEmailID)
	send.Call(a.emailField, 0xc5, 254, 0)
	a.search = a.control("BUTTON", "Search", wsTab, 0, searchContactsID)
	a.matchesList = a.control("LISTBOX", "Matching contacts", wsTab|0x00200101, 0x200, contactMatchesID)
	a.find = a.control("BUTTON", "Look up", wsTab, 0, findContactID)
	a.firstLabel = a.control("STATIC", "First name", 0, 0, 0)
	a.firstName = a.control("EDIT", "", wsTab|0x80, 0x200, contactFirstNameID)
	a.lastLabel = a.control("STATIC", "Last name", 0, 0, 0)
	a.lastName = a.control("EDIT", "", wsTab|0x80, 0x200, contactLastNameID)
	a.addMember = a.control("BUTTON", "Add to list", wsTab, 0, addContactID)
	a.removeMember = a.control("BUTTON", "Remove from list", wsTab, 0, removeContactID)
	a.unsubscribe = a.control("BUTTON", "Unsubscribe email…", wsTab, 0, unsubscribeContactID)
	a.listMembers = a.control("BUTTON", "List members", wsTab, 0, listMembersID)
	a.previousMembers = a.control("BUTTON", "Previous", wsTab, 0, previousMembersID)
	a.nextMembers = a.control("BUTTON", "Next", wsTab, 0, nextMembersID)
	a.memberRange = a.control("STATIC", "", 0, 0, 0)
	a.contactDetails = a.control("EDIT", "", wsTab|0x00200844, 0x200, 0)
}

func (a *application) layoutContacts(set func(uintptr, int, int, int, int), width, height int) {
	top := 0
	if a.switchScreen != 0 {
		set(a.switchScreen, 16, 92, 280, 40)
	} else {
		top = 48
	}
	set(a.contactListLabel, 16, 140-top, width-32, 35)
	set(a.contactLists, 16, 176-top, width-32, 250)
	set(a.emailLabel, 16, 221-top, width-32, 35)
	set(a.emailField, 16, 257-top, width-350, 40)
	set(a.find, width-322, 257-top, 134, 40)
	set(a.search, width-178, 257-top, 162, 40)
	half := (width - 48) / 2
	set(a.firstLabel, 16, 310-top, half, 35)
	set(a.lastLabel, 32+half, 310-top, half, 35)
	set(a.firstName, 16, 345-top, half, 40)
	set(a.lastName, 32+half, 345-top, half, 40)
	buttonWidth := (width - 56) / 3
	set(a.addMember, 16, 399-top, buttonWidth, 42)
	set(a.removeMember, 28+buttonWidth, 399-top, buttonWidth, 42)
	set(a.unsubscribe, 40+2*buttonWidth, 399-top, buttonWidth, 42)
	set(a.listMembers, 16, 452-top, 190, 40)
	set(a.previousMembers, 218, 452-top, 130, 40)
	set(a.nextMembers, 360, 452-top, 105, 40)
	set(a.memberRange, 477, 458-top, width-493, 34)
	detailsWidth := width - 32
	if len(a.matches) > 0 {
		detailsWidth = (width - 48) / 2
		set(a.matchesList, 32+detailsWidth, 505-top, detailsWidth, height-521+top)
	}
	set(a.contactDetails, 16, 505-top, detailsWidth, height-521+top)
}

func (a *application) updateContacts() {
	var visible uintptr
	if a.managing {
		visible = 5
	}
	for _, control := range []uintptr{a.contactListLabel, a.contactLists, a.emailLabel,
		a.emailField, a.find, a.search, a.listMembers, a.previousMembers, a.nextMembers, a.memberRange, a.firstLabel, a.firstName, a.lastLabel, a.lastName, a.addMember, a.removeMember, a.unsubscribe, a.contactDetails} {
		show.Call(control, visible)
	}
	show.Call(a.matchesList, 0)
	if a.managing && len(a.matches) > 0 {
		show.Call(a.matchesList, 5)
	}
	ready := a.managing && a.path != "" && !a.busy
	listID, _ := a.contactList()
	onList := a.selectedContact.onList(listID)
	hasName := a.selectedContact.name() != ""
	enterName := ready && a.contactChecked && !hasName
	canAdd := ready && a.contactChecked && (!onList || !hasName) && (hasName || a.enteredContactName().full() != "")
	label := "Add to list"
	if onList && !hasName {
		label = "Save name"
	}
	caption(a.addMember, label)
	for _, control := range []struct {
		handle uintptr
		active bool
	}{
		{a.switchScreen, a.review == nil && !a.busy},
		{a.contactLists, ready}, {a.emailField, ready},
		{a.matchesList, ready},
		{a.listMembers, ready && listID > 0},
		{a.previousMembers, ready && a.browsingMembers && a.memberOffset > 0},
		{a.nextMembers, ready && a.browsingMembers && a.hasNextMembers},
		{a.search, ready && listID > 0 && strings.TrimSpace(value(a.emailField)) != ""},
		{a.find, ready && listID > 0 && contactEmail(strings.TrimSpace(value(a.emailField)))},
		{a.addMember, canAdd}, {a.firstName, enterName}, {a.lastName, enterName},
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
	a.browsingMembers, a.hasNextMembers, a.memberOffset = false, false, 0
	caption(a.memberRange, "")
	a.matches = nil
	send.Call(a.matchesList, 0x184, 0, 0) // LB_RESETCONTENT
	a.layout()
}

func (a *application) enteredContactName() contactName {
	return contactName{first: value(a.firstName), last: value(a.lastName)}
}

func (a *application) clearContact() {
	a.contactChecked = false
	a.selectedContact = listContact{}
	caption(a.firstName, "")
	caption(a.lastName, "")
	prompt := "Choose a mailing list. Search by name or look up an exact email address."
	if a.path == "" {
		prompt = "Select a valid account profile."
	}
	caption(a.contactDetails, prompt)
}

func (a *application) contactCommand(id, notification uintptr) bool {
	switch {
	case id == switchScreenID && a.switchScreen != 0 && !a.busy && a.review == nil:
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
	case (id == contactFirstNameID || id == contactLastNameID) && notification == 0x300:
		a.updateContacts()
	case id == listMembersID && notification == 0:
		a.loadMemberPage(0)
	case id == previousMembersID && notification == 0 && a.browsingMembers && a.memberOffset > 0:
		a.loadMemberPage(a.memberOffset - memberPageSize)
	case id == nextMembersID && notification == 0 && a.browsingMembers && a.hasNextMembers:
		a.loadMemberPage(a.memberOffset + memberPageSize)
	case id == searchContactsID && notification == 0:
		a.searchContacts()
	case id == contactMatchesID && notification == 1 && !a.busy:
		index, _, _ := send.Call(a.matchesList, 0x188, 0, 0) // LB_GETCURSEL
		if index < uintptr(len(a.matches)) {
			a.choosingMatch = true
			caption(a.emailField, a.matches[index].Email)
			a.choosingMatch = false
			if a.matches[index].Email == "" {
				a.clearContact()
				caption(a.contactDetails, a.matches[index].name()+"\r\nNo email address on file.")
				a.update()
				return true
			}
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
	name := a.enteredContactName()
	var confirmation string
	switch action {
	case addContact:
		if selected.name() != "" {
			name = selected.nameParts()
		}
		if name.full() == "" || (onList && selected.name() != "") {
			return
		}
		if onList {
			confirmation = fmt.Sprintf("Save %s as the name for %s?", name.full(), email)
		} else {
			confirmation = fmt.Sprintf("Add %s (%s) to %s? Confirm that the contact has agreed to receive mail from this list.", name.full(), email, label)
		}
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
			result.err = client.changeContact(ctx, key, email, listID, action, selected, name)
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
	if result.memberPage {
		a.showMemberPage(result)
		return
	}
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
	name := contact.nameParts()
	caption(a.firstName, name.first)
	caption(a.lastName, name.last)
	if contact.ID == 0 {
		caption(a.contactDetails, "No contact found. Enter a first or last name, then click Add to list.")
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
	if len(result.matches) == 0 {
		caption(a.contactDetails, "No matching names on this list.")
		return
	}
	a.populateMatches(result.matches)
	caption(a.contactDetails, fmt.Sprintf("Matches: %d. Select a contact to review their details.", len(a.matches)))
}

func (a *application) populateMatches(matches []listContact) {
	a.matches = matches
	send.Call(a.matchesList, 0x184, 0, 0) // LB_RESETCONTENT
	for _, contact := range matches {
		email := contact.Email
		if email == "" {
			email = "no email address"
		}
		label := wide(strings.TrimSpace(contact.name() + " (" + email + ")"))
		send.Call(a.matchesList, 0x180, 0, ptr(label)) // LB_ADDSTRING
		runtime.KeepAlive(label)
	}
	a.layout()
}

func (a *application) loadMemberPage(offset int) {
	if !a.managing || a.busy || a.path == "" {
		return
	}
	listID, _ := a.contactList()
	if listID <= 0 {
		return
	}
	key, window, done := a.profile.APIKey, a.window, a.contactDone
	a.busy = true
	caption(a.emailField, "")
	a.clearContact()
	caption(a.contactDetails, "Loading list members…")
	a.update()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		matches, next, err := newBrevoClient().listMemberPage(ctx, key, strconv.FormatInt(listID, 10), offset)
		done <- contactResult{memberPage: true, memberOffset: offset, hasNextMembers: next, matches: matches, err: err}
		post.Call(window, wmContactResult, 0, 0)
	}()
}

func (a *application) showMemberPage(result contactResult) {
	if result.err != nil {
		caption(a.contactDetails, "Could not load list members. "+result.err.Error())
		return
	}
	a.browsingMembers, a.memberOffset, a.hasNextMembers = true, result.memberOffset, result.hasNextMembers
	a.populateMatches(result.matches)
	if len(result.matches) == 0 {
		caption(a.memberRange, "")
		caption(a.contactDetails, "No members on this page.")
		return
	}
	caption(a.memberRange, fmt.Sprintf("Members %d–%d", result.memberOffset+1, result.memberOffset+len(result.matches)))
	caption(a.contactDetails, "Select a member to review their details.")
}
