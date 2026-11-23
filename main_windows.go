package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user       = syscall.NewLazyDLL("user32.dll")
	kernel     = syscall.NewLazyDLL("kernel32.dll")
	gdi        = syscall.NewLazyDLL("gdi32.dll")
	create     = user.NewProc("CreateWindowExW")
	send       = user.NewProc("SendMessageW")
	post       = user.NewProc("PostMessageW")
	move       = user.NewProc("MoveWindow")
	show       = user.NewProc("ShowWindow")
	enable     = user.NewProc("EnableWindow")
	setText    = user.NewProc("SetWindowTextW")
	getText    = user.NewProc("GetWindowTextW")
	getTextLen = user.NewProc("GetWindowTextLengthW")
	messageBox = user.NewProc("MessageBoxW")
	app        application
)

const (
	wmDestroy      = 2
	wmSize         = 5
	wmClose        = 16
	wmCommand      = 0x111
	wmResult       = 0x8001
	wmPreview      = 0x8002
	wmSetFont      = 0x30
	cbAdd          = 0x143
	cbGet          = 0x147
	cbReset        = 0x14b
	cbSet          = 0x14e
	wsChildVisible = 0x50000000
	wsTab          = 0x00010000
	cwDefault      = 0x80000000

	profilesID = 10
	reloadID   = 11
	listsID    = 12
	previewID  = 13
	testID     = 14
	sendID     = 15
	backID     = 16
	viewID     = 17
)

type windowClass struct {
	size, style                        uint32
	proc                               uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menu, class                        *uint16
	iconSmall                          uintptr
}
type message struct {
	window         uintptr
	id             uint32
	wParam, lParam uintptr
	time           uint32
	x, y           int32
	private        uint32
}
type rect struct{ left, top, right, bottom int32 }

type attempt struct {
	path  string
	draft draft
	test  bool
}
type outcome struct {
	attempt attempt
	id      int64
	err     error
	packet  packetResult
}

type previewOutcome struct {
	draft   draft
	content string
	err     error
}

type application struct {
	attachmentLabel, attachmentURL, attachFiles, clearFiles uintptr
	packetFiles                                             []packetFile
	packetDigest                                            [32]byte
	contactsUI
	previewDir, previewHTML, previewFile                                                   string
	previewDone                                                                            chan previewOutcome
	window, font                                                                           uintptr
	profileLabel, profiles, reload, audienceLabel, lists, subjectLabel, subject            uintptr
	templateLabel, templates, bodyLabel, body, preview, test, dispatch, back, view, status uintptr
	dir                                                                                    string
	files, ids, templateIDs                                                                []string
	profile                                                                                profile
	path                                                                                   string
	review                                                                                 *draft
	busy                                                                                   bool
	done                                                                                   chan outcome
	attempted                                                                              map[attempt]outcome
}

func wide(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	return p
}
func ptr(p *uint16) uintptr { return uintptr(unsafe.Pointer(p)) }
func caption(hwnd uintptr, s string) {
	p := wide(s)
	setText.Call(hwnd, ptr(p))
	runtime.KeepAlive(p)
}
func value(hwnd uintptr) string {
	n, _, _ := getTextLen.Call(hwnd)
	buf := make([]uint16, n+1)
	getText.Call(hwnd, ptr(&buf[0]), n+1)
	return syscall.UTF16ToString(buf)
}
func add(hwnd uintptr, s string) {
	p := wide(s)
	send.Call(hwnd, cbAdd, 0, ptr(p))
	runtime.KeepAlive(p)
}
func comboIndex(hwnd uintptr) int {
	n, _, _ := send.Call(hwnd, cbGet, 0, 0)
	return int(n)
}
func popup(window uintptr, title, text string, flags uintptr) uintptr {
	body, heading := wide(text), wide(title)
	result, _, _ := messageBox.Call(window, ptr(body), ptr(heading), flags)
	runtime.KeepAlive(body)
	runtime.KeepAlive(heading)
	return result
}

func (a *application) control(class, label string, style, ex, id uintptr) uintptr {
	kind, title := wide(class), wide(label)
	h, _, _ := create.Call(ex, ptr(kind), ptr(title), wsChildVisible|style,
		0, 0, 0, 0, a.window, id, 0, 0)
	runtime.KeepAlive(kind)
	runtime.KeepAlive(title)
	send.Call(h, wmSetFont, a.font, 1)
	return h
}
func (a *application) init() {
	face := wide("Segoe UI")
	a.font, _, _ = gdi.NewProc("CreateFontW").Call(
		uintptr(uint32(^uint32(27))), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, ptr(face))
	runtime.KeepAlive(face)
	if a.font == 0 {
		a.font, _, _ = gdi.NewProc("GetStockObject").Call(17)
	}
	a.profileLabel = a.control("STATIC", "Account profile", 0, 0, 0)
	a.profiles = a.control("COMBOBOX", "", wsTab|0x00200003, 0, profilesID)
	a.reload = a.control("BUTTON", "Reload profiles", wsTab, 0, reloadID)
	a.audienceLabel = a.control("STATIC", "Mailing list", 0, 0, 0)
	a.lists = a.control("COMBOBOX", "", wsTab|0x00200003, 0, listsID)
	a.templateLabel = a.control("STATIC", "Email template", 0, 0, 0)
	a.templates = a.control("COMBOBOX", "", wsTab|0x00200003, 0, 0)
	a.subjectLabel = a.control("STATIC", "Subject", 0, 0, 0)
	a.subject = a.control("EDIT", "", wsTab|0x80, 0x200, 0)
	a.attachmentLabel = a.control("STATIC", "Files", 0, 0, 0)
	a.attachmentURL = a.control("EDIT", "", wsTab|0x880, 0x200, 0)
	a.attachFiles = a.control("BUTTON", "Attach files", wsTab, 0, attachFilesID)
	a.clearFiles = a.control("BUTTON", "Clear", wsTab, 0, clearFilesID)
	a.bodyLabel = a.control("STATIC", "Message", 0, 0, 0)
	a.body = a.control("EDIT", "", wsTab|0x00201044, 0x200, 0)
	send.Call(a.body, 0xc5, 40000, 0)
	a.preview = a.control("BUTTON", "Preview email", wsTab, 0, previewID)
	a.test = a.control("BUTTON", "Send test", wsTab, 0, testID)
	a.dispatch = a.control("BUTTON", "Send to list", wsTab, 0, sendID)
	a.back = a.control("BUTTON", "Edit", wsTab, 0, backID)
	a.view = a.control("BUTTON", "View HTML", wsTab, 0, viewID)
	a.status = a.control("EDIT", "", wsTab|0x00200844, 0x200, 0)
	a.initContacts()
	a.scanProfiles()
	a.layout()
}

func (a *application) layout() {
	var r rect
	user.NewProc("GetClientRect").Call(a.window, uintptr(unsafe.Pointer(&r)))
	width, height := int(r.right), int(r.bottom)
	if width < 260 || height < 200 {
		return
	}
	set := func(h uintptr, x, y, w, ht int) {
		move.Call(h, uintptr(x), uintptr(y), uintptr(max(w, 1)), uintptr(max(ht, 1)), 1)
	}
	const x = 16
	half := (width - 48) / 2
	set(a.profileLabel, x, 10, width-32, 34)
	set(a.profiles, x, 46, width-252, 280)
	set(a.reload, width-226, 45, 210, 40)
	a.layoutContacts(set, width, height)
	y := 49
	set(a.audienceLabel, x, 91+y, half, 34)
	set(a.templateLabel, x+half+16, 91+y, half, 34)
	set(a.lists, x, 127+y, half, 250)
	set(a.templates, x+half+16, 127+y, half, 250)
	set(a.subjectLabel, x, 172+y, width-32, 34)
	set(a.subject, x, 208+y, width-32, 40)
	set(a.attachmentLabel, x, 258+y, 60, 34)
	set(a.attachmentURL, x+64, 254+y, width-366, 40)
	set(a.attachFiles, width-286, 254+y, 160, 40)
	set(a.clearFiles, width-116, 254+y, 100, 40)
	set(a.bodyLabel, x, 299+y, width-32, 34)
	buttonsY := max(415+y, height-160)
	set(a.body, x, 335+y, width-32, buttonsY-347-y)
	buttonWidth := min(195, (width-68)/4)
	set(a.preview, width-211, buttonsY, 195, 43)
	set(a.dispatch, width-16-buttonWidth, buttonsY, buttonWidth, 43)
	set(a.test, width-28-2*buttonWidth, buttonsY, buttonWidth, 43)
	set(a.back, x, buttonsY, buttonWidth, 43)
	set(a.view, x+buttonWidth+12, buttonsY, buttonWidth, 43)
	set(a.status, x, buttonsY+54, width-32, height-buttonsY-64)
}

func (a *application) update() {
	review := a.review != nil
	for _, item := range []struct {
		hwnd    uintptr
		visible bool
	}{
		{a.preview, !review && !a.managing}, {a.back, review && !a.managing},
		{a.test, review && !a.managing}, {a.dispatch, review && !a.managing}, {a.view, review && !a.managing},
		{a.audienceLabel, !a.managing}, {a.lists, !a.managing}, {a.templateLabel, !a.managing},
		{a.templates, !a.managing}, {a.subjectLabel, !a.managing}, {a.subject, !a.managing},
		{a.attachmentLabel, !a.managing}, {a.attachmentURL, !a.managing},
		{a.attachFiles, !a.managing && !review}, {a.clearFiles, !a.managing && !review},
		{a.bodyLabel, !a.managing}, {a.body, !a.managing}, {a.status, !a.managing},
	} {
		var state uintptr
		if item.visible {
			state = 5
		}
		show.Call(item.hwnd, state)
	}
	for _, item := range []struct {
		hwnd   uintptr
		active bool
	}{
		{a.attachFiles, !review && !a.busy}, {a.clearFiles, !review && !a.busy && len(a.packetFiles) > 0},
		{a.profiles, !review && !a.busy}, {a.reload, !review && !a.busy}, {a.lists, !review && !a.busy && a.path != ""},
		{a.templates, !review && !a.busy && a.path != ""}, {a.preview, !review && !a.busy && a.path != ""}, {a.view, !a.busy}, {a.back, !a.busy}, {a.test, !a.busy}, {a.dispatch, !a.busy},
	} {
		var state uintptr
		if item.active {
			state = 1
		}
		enable.Call(item.hwnd, state)
	}
	var readonly uintptr
	if review || a.busy {
		readonly = 1
	}
	send.Call(a.subject, 0xcf, readonly, 0)
	if review && len(a.packetFiles) > 0 {
		caption(a.dispatch, "Send packet")
	} else {
		caption(a.dispatch, "Send to list")
	}
	send.Call(a.body, 0xcf, readonly, 0)
	a.updateContacts()
}

func (a *application) scanProfiles() {
	oldPath := a.path
	a.path = ""
	a.files = nil
	a.ids = nil
	a.resetContactLists()
	send.Call(a.profiles, cbReset, 0, 0)
	send.Call(a.lists, cbReset, 0, 0)
	send.Call(a.templates, cbReset, 0, 0)
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		caption(a.status, "Could not read the folder that contains the app.")
		a.update()
		return
	}
	selected := 0
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if entry.IsDir() || !strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".example.json") {
			continue
		}
		path := filepath.Join(a.dir, name)
		if path == oldPath || (oldPath == "" && lower == "club-mail.json") {
			selected = len(a.files)
		}
		a.files = append(a.files, path)
		add(a.profiles, name)
	}
	if len(a.files) == 0 {
		caption(a.status, "Copy club-mail.example.json to club-mail.json next to this app, fill it in, then click Reload profiles.")
		a.update()
		return
	}
	send.Call(a.profiles, cbSet, uintptr(selected), 0)
	a.openProfile()
}

func (a *application) openProfile() {
	idx := comboIndex(a.profiles)
	if idx < 0 || idx >= len(a.files) {
		return
	}
	path := a.files[idx]
	p, err := loadProfile(path)
	send.Call(a.lists, cbReset, 0, 0)
	send.Call(a.templates, cbReset, 0, 0)
	a.path = ""
	a.profile = profile{}
	a.ids, a.templateIDs = nil, nil
	if err != nil {
		a.resetContactLists()
		caption(a.status, "Profile: "+err.Error())
		a.update()
		return
	}
	a.path, a.profile, a.ids = path, p, sortedIDs(p.Lists)
	for _, id := range a.ids {
		add(a.lists, p.Lists[id]+" (#"+id+")")
	}
	if len(a.ids) == 1 {
		send.Call(a.lists, cbSet, 0, 0)
	}
	a.templateIDs = sortedIDs(p.Templates)
	if strings.TrimSpace(p.Footer) != "" {
		a.templateIDs = append(a.templateIDs, "")
	}
	for _, id := range a.templateIDs {
		label := p.Templates[id]
		if id == "" {
			label = "Plain email"
		}
		add(a.templates, label)
	}
	// Select a lone template or the plain email option. Multiple templates require a choice.
	if len(a.templateIDs) == 1 || len(p.Templates) == 1 {
		send.Call(a.templates, cbSet, 0, 0)
	}
	a.resetContactLists()
	caption(a.status, p.Label+"   |   From "+p.Sender.Email)
	a.update()
}

func (a *application) startReview() {
	if a.busy || a.review != nil {
		return
	}
	i := comboIndex(a.lists)
	var id string
	if i >= 0 && i < len(a.ids) {
		id = a.ids[i]
	}
	templateIndex := comboIndex(a.templates)
	if templateIndex < 0 || templateIndex >= len(a.templateIDs) {
		popup(a.window, "Preview email", "Choose an email template or Plain email.", 0x30)
		return
	}
	d := draft{ListID: id, Subject: strings.TrimSpace(value(a.subject)),
		Body:         strings.TrimSpace(strings.ReplaceAll(value(a.body), "\r\n", "\n")),
		Template:     a.templateIDs[templateIndex],
		PacketDigest: a.packetDigest}
	if err := d.validate(a.profile); err != nil {
		popup(a.window, "Preview email", err.Error(), 0x30)
		return
	}
	a.busy = true
	a.update()
	caption(a.status, "Preparing HTML preview…")
	p := a.profile
	go func() {
		content, err := newBrevoClient().prepare(context.Background(), p, d)
		a.previewDone <- previewOutcome{d, content, err}
		post.Call(a.window, wmPreview, 0, 0)
	}()
}

func (a *application) showReview(result previewOutcome) {
	a.busy = false
	if result.err != nil {
		a.update()
		caption(a.status, result.err.Error())
		return
	}
	d := result.draft
	a.review = &d
	a.previewHTML = result.content
	a.previewFile = ""
	caption(a.bodyLabel, "Email preview")
	body := d.Body
	if d.Template == "" {
		body += "\n\n" + a.profile.Footer + "\nUnsubscribe"
	} else {
		caption(a.bodyLabel, "Message preview · "+a.profile.Templates[d.Template])
	}
	caption(a.body, strings.ReplaceAll(body, "\n", "\r\n"))
	caption(a.status, "From "+a.profile.Sender.Email+"   |   Reply to "+a.profile.ReplyTo+
		"\r\nTest recipient: "+a.profile.TestEmail)
	a.update()
	if err := a.openPreview(); err != nil {
		caption(a.status, err.Error())
	}
}

func (a *application) openPreview() error {
	if a.review == nil {
		return nil
	}
	if a.previewDir == "" {
		dir, err := os.MkdirTemp("", "Brevo-Club-Mail-preview-")
		if err != nil {
			return fmt.Errorf("Could not create a temporary preview folder.")
		}
		a.previewDir = dir
	}
	if a.previewFile == "" {
		file, err := os.CreateTemp(a.previewDir, "email-*.html")
		if err != nil {
			return fmt.Errorf("Could not create the HTML preview file.")
		}
		_, writeErr := file.WriteString(browserPreview(a.review.Subject, a.previewHTML))
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(file.Name())
			return fmt.Errorf("Could not save the HTML preview file.")
		}
		a.previewFile = file.Name()
	}

	verb, path := wide("open"), wide(a.previewFile)
	result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		a.window, ptr(verb), ptr(path), 0, 0, 1)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(path)
	if result <= 32 {
		return fmt.Errorf("Could not open the preview in the default browser.")
	}
	return nil
}

func (a *application) edit() {
	d := *a.review
	a.review = nil
	a.previewHTML, a.previewFile = "", ""
	caption(a.bodyLabel, "Message")
	a.update()
	previous, ok := a.attempted[attempt{a.path, d, false}]
	if ok && previous.err == nil {
		d.Subject, d.Body = "", ""
		a.clearPacketFiles()
	}
	caption(a.subject, d.Subject)
	caption(a.body, strings.ReplaceAll(d.Body, "\n", "\r\n"))
	caption(a.status, a.profile.Label+"   |   From "+a.profile.Sender.Email)
}

func (a *application) submit(test bool) {
	if a.review == nil || a.busy {
		return
	}
	key := attempt{a.path, *a.review, test}
	if prev, ok := a.attempted[key]; ok {
		a.displayResult(prev)
		return
	}
	label := a.profile.Lists[key.draft.ListID] + " (#" + key.draft.ListID + ")"
	prompt := "Send this email to " + label + "?\r\n\r\nSubject: " + key.draft.Subject
	if test {
		prompt = "Send a test only to " + a.profile.TestEmail + "?"
	}
	files := a.packetFiles
	if len(files) != 0 {
		if !test {
			testKey := key
			testKey.test = true
			if previous, ok := a.attempted[testKey]; !ok || previous.err != nil {
				popup(a.window, "Send packet", "Send a successful test of this meeting packet first.", 0x30)
				return
			}
		}
		prompt += "\r\n\r\nAttachments: " + packetNames(files)
	}
	if popup(a.window, "Confirm send", prompt, 0x124) != 6 {
		return
	}
	a.busy = true
	a.update()
	caption(a.status, "Submitting to Brevo…")
	p, content := a.profile, a.previewHTML
	go func() {
		client := newBrevoClient()
		result := outcome{attempt: key}
		if len(files) > 0 {
			result.packet, result.err = client.sendPacket(context.Background(), p, key.draft, content, files, test)
		} else {
			result.id, result.err = client.submit(context.Background(), p, key.draft, content, test)
		}
		a.done <- result
		post.Call(a.window, wmResult, 0, 0)
	}()
}

func (a *application) displayResult(result outcome) {
	if result.attempt.draft.PacketDigest != ([32]byte{}) {
		message := fmt.Sprintf("Brevo accepted %d of %d packet emails.", result.packet.accepted, result.packet.total)
		if result.err != nil {
			message = result.err.Error()
			if result.packet.started {
				message = fmt.Sprintf("Brevo confirmed %d of %d packet emails before a send could not be confirmed. Check the transactional logs before resending.\r\n%s", result.packet.accepted, result.packet.total, message)
			}
		}
		caption(a.status, message)
		return
	}
	text := "Brevo accepted the campaign for sending."
	if result.attempt.test {
		text = "Brevo accepted the test email."
	}
	if result.err != nil {
		text = result.err.Error()
		if result.id > 0 {
			text += "\r\nCheck Brevo before trying again."
		}
	}
	if result.id > 0 {
		text += fmt.Sprintf(" Campaign ID: %d.", result.id)
	}
	caption(a.status, text)
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmSize:
		if app.status != 0 {
			app.layout()
		}
	case wmCommand:
		id, notification := wParam&0xffff, (wParam>>16)&0xffff
		switch {
		case app.contactCommand(id, notification):
		case id == profilesID && notification == 1 && !app.busy && app.review == nil:
			app.openProfile()
		case id == reloadID && !app.busy && app.review == nil:
			app.scanProfiles()
		case id == attachFilesID && !app.busy && app.review == nil:
			app.attachPacketFiles()
		case id == clearFilesID && !app.busy && app.review == nil:
			app.clearPacketFiles()
		case id == previewID:
			app.startReview()
		case id == viewID && !app.busy:
			if err := app.openPreview(); err != nil {
				caption(app.status, err.Error())
			}
		case id == testID:
			app.submit(true)
		case id == sendID:
			app.submit(false)
		case id == backID && app.review != nil && !app.busy:
			app.edit()
		}
	case wmContactResult:
		app.finishContact(<-app.contactDone)
	case wmPreview:
		app.showReview(<-app.previewDone)
	case wmResult:
		result := <-app.done
		app.busy = false
		if result.err == nil || result.id > 0 || result.packet.started {
			app.attempted[result.attempt] = result
		}
		app.update()
		app.displayResult(result)
	case wmClose:
		if app.busy {
			popup(hwnd, "Brevo Club Mail", "Wait for the current operation to finish.", 0x30)
			return 0
		}
	case wmDestroy:
		user.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	res, _, _ := user.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wParam, lParam)
	return res
}

type monitorInfo struct {
	size          uint32
	monitor, work rect
	flags         uint32
}

func sizeWindow(hwnd uintptr) {
	monitor, _, _ := user.NewProc("MonitorFromWindow").Call(hwnd, 2)
	var info monitorInfo
	info.size = uint32(unsafe.Sizeof(info))
	ok, _, _ := user.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return
	}
	workWidth := info.work.right - info.work.left
	workHeight := info.work.bottom - info.work.top
	width := min(workWidth, max(620, workWidth*3/4))
	height := min(workHeight, max(500, workHeight*3/4))
	x := info.work.left + (workWidth-width)/2
	y := info.work.top + (workHeight-height)/2
	move.Call(hwnd, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 1)
}

func main() {
	runtime.LockOSThread()
	executable, err := os.Executable()
	if err != nil {
		popup(0, "Brevo Club Mail", "Could not locate the app folder.", 0x10)
		return
	}
	app.dir = filepath.Dir(executable)
	app.done = make(chan outcome, 1)
	app.previewDone = make(chan previewOutcome, 1)
	defer func() {
		if app.previewDir != "" {
			_ = os.RemoveAll(app.previewDir)
		}
	}()
	app.attempted = make(map[attempt]outcome)
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	name := wide("BrevoClubMail")
	wc := windowClass{proc: syscall.NewCallback(windowProc), instance: instance,
		cursor: cursor, background: 16, class: name}
	wc.size = uint32(unsafe.Sizeof(wc))
	registered, _, _ := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if registered == 0 {
		popup(0, "Brevo Club Mail", "Could not register the app window.", 0x10)
		return
	}
	title := wide("Brevo Club Mail")
	hwnd, _, _ := create.Call(0x10000, ptr(name), ptr(title), 0x00cf0000, cwDefault, cwDefault,
		1090, 720, 0, 0, instance, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		popup(0, "Brevo Club Mail", "Could not open the app window.", 0x10)
		return
	}
	app.window = hwnd
	sizeWindow(hwnd)
	app.init()
	show.Call(hwnd, 5)
	var msg message
	for {
		rc, _, _ := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(rc) <= 0 {
			break
		}
		handled, _, _ := user.NewProc("IsDialogMessageW").Call(hwnd, uintptr(unsafe.Pointer(&msg)))
		if handled == 0 {
			user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
			user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}
