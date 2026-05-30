package ui

import (
	"context"
	"fmt"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/igorivitskyy/lazyssh/internal/config"
	"github.com/igorivitskyy/lazyssh/internal/model"
	sshpkg "github.com/igorivitskyy/lazyssh/internal/ssh"
)

// App is the top-level application object.
type App struct {
	tApp      *tview.Application
	pages     *tview.Pages
	store     *model.Store
	masterPwd string // cached for the session; empty until first use

	hostList *hostList // main page reference for tree refresh
}

// NewApp constructs the application but does not start the event loop.
func NewApp(store *model.Store) *App {
	a := &App{
		tApp:  tview.NewApplication(),
		pages: tview.NewPages(),
		store: store,
	}

	a.tApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Global Ctrl+C / Ctrl+Q → quit (tview handles Ctrl+C by default,
		// but we add Ctrl+Q as a convenience).
		if event.Key() == tcell.KeyCtrlQ {
			a.tApp.Stop()
			return nil
		}
		return event
	})

	hl := newHostList(a)
	a.hostList = hl
	a.pages.AddPage("hosts", hl.root, true, true)

	a.tApp.SetRoot(a.pages, true).EnableMouse(false)
	return a
}

// Run starts the TUI event loop. Blocks until the user quits.
func (a *App) Run() error {
	return a.tApp.Run()
}

// save persists the store to disk and shows an error modal on failure.
func (a *App) save() {
	if err := config.Save(a.store); err != nil {
		a.showError("Save failed: " + err.Error())
	}
}

// --- page navigation --------------------------------------------------------

func (a *App) showPage(name string) {
	a.pages.SwitchToPage(name)
}

func (a *App) openHostForm(host *model.Host) {
	form := newHostForm(a, host)
	a.pages.AddPage("form", form.root, true, true)
	a.pages.SwitchToPage("form")
	a.tApp.SetFocus(form.form)
}

func (a *App) closeHostForm() {
	a.pages.RemovePage("form")
	a.pages.SwitchToPage("hosts")
	a.hostList.rebuild()
	a.tApp.SetFocus(a.hostList.tree)
}

func (a *App) openFileBrowser(host *model.Host) {
	a.showInfo("Connecting to " + host.UserHost() + "…")

	a.connectSFTP(host, func(client *sshpkg.Client) {
		fb := newFileBrowser(a, host, client)
		a.pages.AddPage("browser", fb.root, true, true)
		a.pages.SwitchToPage("browser")
		a.tApp.SetFocus(fb.localTable)
	})
}

func (a *App) closeFileBrowser() {
	a.pages.RemovePage("browser")
	a.pages.SwitchToPage("hosts")
	a.tApp.SetFocus(a.hostList.tree)
}

// --- SFTP connection (background goroutine) ---------------------------------

func (a *App) connectSFTP(host *model.Host, onSuccess func(*sshpkg.Client)) {
	a.connectSFTPwithPwd(host, "", false, onSuccess)
}

// connectSFTPwithPwd is the internal implementation.
// forcePwd overrides any stored password; retried=true suppresses a second
// password prompt so we don't loop forever on a wrong password.
func (a *App) connectSFTPwithPwd(host *model.Host, forcePwd string, retried bool, onSuccess func(*sshpkg.Client)) {
	go func() {
		var password string

		if forcePwd != "" {
			password = forcePwd
		} else if host.AuthType == model.AuthTypePassword && host.Password != "" {
			if config.IsEncrypted(host.Password) {
				if a.masterPwd == "" {
					a.tApp.QueueUpdateDraw(func() {
						a.pages.RemovePage("modal")
						a.promptMasterPassword(func(pwd string) {
							a.masterPwd = pwd
							a.connectSFTPwithPwd(host, "", false, onSuccess)
						})
					})
					return
				}
				var err error
				password, err = config.Decrypt(a.masterPwd, host.Password)
				if err != nil {
					a.masterPwd = ""
					a.tApp.QueueUpdateDraw(func() {
						a.showError("Decrypt password: " + err.Error())
					})
					return
				}
			} else {
				password = host.Password
			}
		}

		unknownHostCh := make(chan bool, 1)

		client, err := sshpkg.Connect(host, password, func(hostname, fingerprint string) bool {
			a.tApp.QueueUpdateDraw(func() {
				a.showUnknownHostModal(hostname, fingerprint, unknownHostCh)
			})
			return <-unknownHostCh
		})

		a.tApp.QueueUpdateDraw(func() {
			a.removeModal()
			if err != nil {
				// If key auth failed and we haven't asked for a password yet,
				// offer a password prompt and retry once.
				if sshpkg.IsAuthError(err) && !retried {
					a.promptInputModal(
						"Authentication Failed",
						"Password for "+host.UserHost(),
						"",
						func(pwd string) {
							if pwd == "" {
								a.showError("Auth failed: " + err.Error())
								return
							}
							a.showInfo("Connecting to " + host.UserHost() + "…")
							a.connectSFTPwithPwd(host, pwd, true, onSuccess)
						},
					)
					return
				}
				a.showError("Connect: " + err.Error())
			} else {
				onSuccess(client)
			}
		})
	}()
}

// --- modal helpers ----------------------------------------------------------

const modalPage = "modal"

func (a *App) removeModal() {
	if p, _ := a.pages.GetFrontPage(); p == modalPage {
		a.pages.RemovePage(modalPage)
	}
}

// showInfo shows a non-interactive informational overlay (no buttons).
func (a *App) showInfo(msg string) {
	m := tview.NewModal().SetText(msg)
	a.pages.AddPage(modalPage, m, true, true)
}

// showUpdatableInfo shows a progress modal with a Cancel button.
// Must be called from the UI goroutine.
// Returns:
//   - updateFn  – update the modal text (goroutine-safe, no-op after cancel)
//   - closeFn   – close the modal when the operation finishes (goroutine-safe, idempotent)
//   - ctx       – cancelled when the user presses Cancel or closeFn is called
func (a *App) showUpdatableInfo(initialText string) (updateFn func(string), closeFn func(), ctx context.Context) {
	cancelCtx, cancel := context.WithCancel(context.Background())
	modal := tview.NewModal().
		SetText(initialText).
		AddButtons([]string{"Cancel"}).
		SetDoneFunc(func(_ int, _ string) {
			cancel()
			a.removeModal()
		})
	a.pages.AddPage(modalPage, modal, true, true)

	var once sync.Once
	closeFn = func() {
		once.Do(func() {
			cancel()
			a.tApp.QueueUpdateDraw(func() { a.removeModal() })
		})
	}
	updateFn = func(text string) {
		select {
		case <-cancelCtx.Done():
		default:
			a.tApp.QueueUpdateDraw(func() { modal.SetText(text) })
		}
	}
	ctx = cancelCtx
	return
}

// showError shows an error modal with an OK button.
func (a *App) showError(msg string) {
	m := tview.NewModal().
		SetText("⚠  " + msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) { a.removeModal() })
	a.pages.AddPage(modalPage, m, true, true)
	a.tApp.SetFocus(m)
}

// showConfirm shows a Yes/No confirmation modal.
func (a *App) showConfirm(msg string, onYes func()) {
	m := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(_ int, label string) {
			a.removeModal()
			if label == "Yes" && onYes != nil {
				onYes()
			}
		})
	a.pages.AddPage(modalPage, m, true, true)
	a.tApp.SetFocus(m)
}

// showUnknownHostModal asks the user whether to trust an unrecognised host key.
// The user's answer is sent on ch.
func (a *App) showUnknownHostModal(hostname, fingerprint string, ch chan<- bool) {
	msg := fmt.Sprintf(
		"Unknown host: [yellow]%s[-]\n\nFingerprint (SHA-256):\n[green]%s[-]\n\nAdd to known_hosts and trust?",
		hostname, fingerprint,
	)
	m := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"Trust", "Abort"}).
		SetDoneFunc(func(_ int, label string) {
			a.removeModal()
			ch <- (label == "Trust")
		})
	a.pages.AddPage(modalPage, m, true, true)
	a.tApp.SetFocus(m)
}

// promptMasterPassword shows an input form asking for the master password.
func (a *App) promptMasterPassword(onOK func(string)) {
	var pwd string
	form := tview.NewForm().
		AddPasswordField("Master Password", "", 40, '*', func(t string) { pwd = t }).
		AddButton("OK", func() {
			a.pages.RemovePage("masterpwd")
			onOK(pwd)
		}).
		AddButton("Cancel", func() {
			a.pages.RemovePage("masterpwd")
		})
	form.SetBorder(true).
		SetTitle(" Master Password ").
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorYellow)

	a.pages.AddPage("masterpwd", centeredBox(form, 52, 9), true, true)
	a.tApp.SetFocus(form)
}

// promptInputModal shows a single-field input modal and calls onOK with the value.
func (a *App) promptInputModal(title, label, initial string, onOK func(string)) {
	const page = "input"
	var val string
	form := tview.NewForm().
		AddInputField(label, initial, 50, nil, func(t string) { val = t }).
		AddButton("OK", func() {
			a.pages.RemovePage(page)
			onOK(val)
		}).
		AddButton("Cancel", func() {
			a.pages.RemovePage(page)
		})
	form.SetBorder(true).
		SetTitle(" " + title + " ").
		SetTitleAlign(tview.AlignCenter)

	a.pages.AddPage(page, centeredBox(form, 60, 9), true, true)
	a.tApp.SetFocus(form)
}

// --- layout helpers ---------------------------------------------------------

// centeredBox wraps p in a Flex that centres it at the given dimensions.
func centeredBox(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(tview.NewBox(), 0, 1, false).
				AddItem(p, height, 1, true).
				AddItem(tview.NewBox(), 0, 1, false),
			width, 1, true,
		).
		AddItem(tview.NewBox(), 0, 1, false)
}
