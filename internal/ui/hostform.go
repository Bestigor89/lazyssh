package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/Bestigor89/lazyssh/internal/config"
	"github.com/Bestigor89/lazyssh/internal/model"
)

// hostForm is the add/edit host page.
type hostForm struct {
	root   *tview.Flex
	form   *tview.Form
	app    *App
	target *model.Host // nil → create mode, non-nil → edit mode

	// field values (kept in sync via onChange callbacks)
	fName     string
	fHostname string
	fPort     string
	fUser     string
	fAuthType string // "key" or "password"
	fKeyPath  string
	fPassword string
	fFolder   string
	fTags     string
}

func newHostForm(a *App, existing *model.Host) *hostForm {
	hf := &hostForm{app: a, target: existing, fAuthType: model.AuthTypeKey}

	// Pre-populate fields when editing.
	if existing != nil {
		hf.fName = existing.Name
		hf.fHostname = existing.Hostname
		hf.fPort = strconv.Itoa(existing.EffectivePort())
		hf.fUser = existing.User
		hf.fAuthType = existing.AuthType
		if hf.fAuthType == "" {
			hf.fAuthType = model.AuthTypeKey
		}
		hf.fKeyPath = existing.KeyPath
		// Don't pre-fill password (it may be encrypted; show placeholder instead).
		hf.fFolder = existing.Folder
		hf.fTags = strings.Join(existing.Tags, ", ")
	} else {
		hf.fPort = "22"
		hf.fAuthType = model.AuthTypeKey
	}

	title := " Add Host "
	if existing != nil {
		title = " Edit Host "
	}

	hf.form = tview.NewForm()
	hf.form.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorCornflowerBlue)

	authIdx := 0
	if hf.fAuthType == model.AuthTypePassword {
		authIdx = 1
	}

	hf.form.
		AddInputField("Name", hf.fName, 32, nil, func(t string) { hf.fName = t }).
		AddInputField("Hostname / IP", hf.fHostname, 32, nil, func(t string) { hf.fHostname = t }).
		AddInputField("Port", hf.fPort, 6, acceptDigits, func(t string) { hf.fPort = t }).
		AddInputField("User", hf.fUser, 32, nil, func(t string) { hf.fUser = t }).
		AddDropDown("Auth Type", []string{model.AuthTypeKey, model.AuthTypePassword}, authIdx,
			func(option string, _ int) { hf.fAuthType = option }).
		AddInputField("Key Path", hf.fKeyPath, 32, nil, func(t string) { hf.fKeyPath = t }).
		AddPasswordField("Password", "", 32, '*', func(t string) { hf.fPassword = t }).
		AddInputField("Folder", hf.fFolder, 32, nil, func(t string) { hf.fFolder = t }).
		AddInputField("Tags (comma-sep)", hf.fTags, 32, nil, func(t string) { hf.fTags = t }).
		AddButton("Save", hf.save).
		AddButton("Cancel", func() { a.closeHostForm() })

	// Center the form horizontally.
	hf.root = tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(tview.NewBox(), 0, 1, false).
				AddItem(hf.form, 26, 1, true).
				AddItem(tview.NewBox(), 0, 1, false),
			62, 1, true,
		).
		AddItem(tview.NewBox(), 0, 1, false)

	return hf
}

// save validates and persists the host.
func (hf *hostForm) save() {
	name := strings.TrimSpace(hf.fName)
	hostname := strings.TrimSpace(hf.fHostname)
	user := strings.TrimSpace(hf.fUser)

	if name == "" {
		hf.app.showError("Name is required")
		return
	}
	if hostname == "" {
		hf.app.showError("Hostname is required")
		return
	}
	if user == "" {
		hf.app.showError("User is required")
		return
	}

	port := 22
	if p, err := strconv.Atoi(strings.TrimSpace(hf.fPort)); err == nil && p > 0 {
		port = p
	}

	var tags []string
	for _, t := range strings.Split(hf.fTags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	// Build or update the host.
	var h *model.Host
	if hf.target != nil {
		h = hf.target
	} else {
		h = &model.Host{ID: model.NewID()}
		hf.app.store.Hosts = append(hf.app.store.Hosts, h)
	}

	h.Name = name
	h.Hostname = hostname
	h.Port = port
	h.User = user
	h.AuthType = hf.fAuthType
	h.KeyPath = strings.TrimSpace(hf.fKeyPath)
	h.Folder = strings.TrimSpace(hf.fFolder)
	h.Tags = tags

	// Handle password:
	//   - empty input and edit mode → keep existing value
	//   - non-empty → encrypt if master password set, else store plain
	newPwd := hf.fPassword
	if newPwd != "" {
		if hf.app.masterPwd != "" {
			if encrypted, err := config.Encrypt(hf.app.masterPwd, newPwd); err == nil {
				h.Password = encrypted
			} else {
				hf.app.showError(fmt.Sprintf("Encrypt password: %v", err))
				return
			}
		} else {
			// No master password set — store plain text with a warning.
			// A future save will encrypt it once the user sets a master password.
			h.Password = newPwd
		}
	}
	// If newPwd == "" and we're editing, h.Password keeps its old value (already assigned via pointer).

	hf.app.save()
	hf.app.closeHostForm()
}

// acceptDigits is an input acceptance function that allows only digit characters.
func acceptDigits(text string, c rune) bool {
	return c >= '0' && c <= '9'
}
