package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/Bestigor89/lazyssh/internal/config"
	"github.com/Bestigor89/lazyssh/internal/model"
)

// hostList is the main screen: a tree of saved hosts grouped by folder.
type hostList struct {
	root       *tview.Flex
	tree       *tview.TreeView
	statusBar  *tview.TextView
	searchBar  *tview.InputField
	searching  bool
	app        *App
	filterText string
}

func newHostList(a *App) *hostList {
	hl := &hostList{app: a}

	// ── Title bar ──────────────────────────────────────────────────────────
	title := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[black::b] LazySSH [-::-]  [darkgray]" + Version + "[-]")
	title.SetBackgroundColor(tcell.ColorDodgerBlue)

	// ── Tree ───────────────────────────────────────────────────────────────
	hl.tree = tview.NewTreeView()
	hl.tree.SetBorder(false)
	hl.tree.SetGraphics(true)
	hl.tree.SetTopLevel(1) // hide the invisible root node

	// ── Status bar ─────────────────────────────────────────────────────────
	hl.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(statusText())
	hl.statusBar.SetBackgroundColor(tcell.ColorDarkSlateGray)

	// ── Search bar (hidden until '/' is pressed) ────────────────────────────
	hl.searchBar = tview.NewInputField().
		SetLabel(" / Search: ").
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorDarkBlue)
	hl.searchBar.SetChangedFunc(func(text string) {
		hl.filterText = text
		hl.rebuild()
	})
	hl.searchBar.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter, tcell.KeyTab:
			// Keep the filter and move focus to the results.
			hl.focusResults()
		default: // Escape
			hl.hideSearch()
		}
	})
	hl.searchBar.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Navigation keys move the selection in the filtered tree while the
		// cursor stays in the search field, so the user can keep typing.
		switch event.Key() {
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn:
			hl.tree.InputHandler()(event, func(tview.Primitive) {})
			return nil
		}
		return event
	})

	// ── Layout ─────────────────────────────────────────────────────────────
	hl.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(hl.tree, 0, 1, true).
		AddItem(hl.statusBar, 2, 0, false)

	hl.rebuild()
	hl.bindKeys()
	return hl
}

// rebuild recreates the tree from the current store, applying filterText.
func (hl *hostList) rebuild() {
	// Remember the selected host so the cursor survives the rebuild.
	prevHost, _ := nodeHost(hl.tree.GetCurrentNode())

	root := tview.NewTreeNode("Hosts").SetSelectable(false)
	hl.tree.SetRoot(root).SetCurrentNode(root)

	// folderNodes caches folder nodes by their full path (e.g. "prod/web").
	folderNodes := map[string]*tview.TreeNode{}

	// getFolder returns the TreeNode for the given folder path, creating
	// intermediate nodes as needed.
	var getFolder func(path string) *tview.TreeNode
	getFolder = func(path string) *tview.TreeNode {
		if path == "" {
			return root
		}
		if n, ok := folderNodes[path]; ok {
			return n
		}
		// Ensure parent exists.
		slashIdx := strings.LastIndex(path, "/")
		parentPath := ""
		name := path
		if slashIdx >= 0 {
			parentPath = path[:slashIdx]
			name = path[slashIdx+1:]
		}
		parent := getFolder(parentPath)

		node := tview.NewTreeNode("📁 " + name).
			SetSelectable(true).
			SetColor(tcell.ColorYellow).
			SetReference("folder:" + path).
			SetExpanded(true)
		parent.AddChild(node)
		folderNodes[path] = node
		return node
	}

	filter := strings.ToLower(hl.filterText)

	for _, h := range hl.app.store.Hosts {
		if filter != "" && !hostMatchesFilter(h, filter) {
			continue
		}

		parent := getFolder(h.Folder)

		authIcon := "🔑"
		if h.AuthType == model.AuthTypePassword {
			authIcon = "🔒"
		}
		label := fmt.Sprintf("%s [white::b]%s[-::-]  [cyan]%s:%d[-]",
			authIcon, h.Name, h.Hostname, h.EffectivePort())
		if len(h.Tags) > 0 {
			label += "  [yellow]" + strings.Join(h.Tags, " ") + "[-]"
		}

		selectedStyle := tcell.StyleDefault.
			Background(tcell.ColorDodgerBlue).
			Foreground(tcell.ColorWhite)
		node := tview.NewTreeNode(label).
			SetSelectable(true).
			SetReference(h).
			SetSelectedTextStyle(selectedStyle)
		parent.AddChild(node)
	}

	// If there are no children at all, show a hint.
	if len(root.GetChildren()) == 0 {
		hint := tview.NewTreeNode("[gray]No hosts. Press [yellow]a[-] to add one.[-]").
			SetSelectable(false)
		root.AddChild(hint)
	}

	hl.selectAfterRebuild(root, prevHost)
}

// selectAfterRebuild puts the cursor on prev if it is still in the tree,
// otherwise on the first host (when filtering) or the first selectable node.
func (hl *hostList) selectAfterRebuild(root *tview.TreeNode, prev *model.Host) {
	var firstHost, firstAny, match *tview.TreeNode
	root.Walk(func(node, _ *tview.TreeNode) bool {
		if node.GetReference() == nil { // root and the "no hosts" hint
			return true
		}
		if firstAny == nil {
			firstAny = node
		}
		if h, ok := nodeHost(node); ok {
			if firstHost == nil {
				firstHost = node
			}
			if prev != nil && h.ID == prev.ID {
				match = node
			}
		}
		return true
	})

	switch {
	case match != nil:
		hl.tree.SetCurrentNode(match)
	case hl.filterText != "" && firstHost != nil:
		hl.tree.SetCurrentNode(firstHost)
	case firstAny != nil:
		hl.tree.SetCurrentNode(firstAny)
	}
}

// bindKeys attaches keyboard handlers to the tree widget.
func (hl *hostList) bindKeys() {
	hl.tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Typing '/' opens the search bar.
		if event.Key() == tcell.KeyRune && event.Rune() == '/' {
			hl.showSearch()
			return nil
		}
		if event.Key() == tcell.KeyTab && hl.searching {
			// Back to the search field to refine the filter.
			hl.focusSearch()
			return nil
		}

		node := hl.tree.GetCurrentNode()
		host, _ := nodeHost(node)

		switch event.Key() {
		case tcell.KeyEnter:
			if host != nil {
				// Enter → file browser (default action).
				hl.app.openFileBrowser(host)
				return nil
			}
			// Enter on a folder node — toggle expand.
			if node != nil {
				node.SetExpanded(!node.IsExpanded())
			}
			return nil

		case tcell.KeyRune:
			switch event.Rune() {
			case 'a':
				hl.app.openHostForm(nil)
				return nil
			case 'e':
				if host != nil {
					hl.app.openHostForm(host)
				}
				return nil
			case 'd':
				if host != nil {
					hl.confirmDelete(host)
				}
				return nil
			case 'f':
				if host != nil {
					hl.app.openFileBrowser(host)
				}
				return nil
			case 's':
				// Ask: persistent sessions via lss, or plain ssh.
				if host != nil {
					hl.app.chooseSSHMode(host)
				}
				return nil
			case 'S':
				// Shortcut: plain ssh, skipping the lss helper entirely.
				if host != nil {
					hl.app.openPlainSSH(host)
				}
				return nil
			case 'E':
				hl.doExport()
				return nil
			case 'I':
				hl.doImport()
				return nil
			case 'q':
				hl.app.tApp.Stop()
				return nil
			}
		case tcell.KeyEscape:
			if hl.searching {
				hl.hideSearch()
				return nil
			}
		}
		return event
	})
}

// --- search -----------------------------------------------------------------

func (hl *hostList) showSearch() {
	if !hl.searching {
		hl.searching = true
		// Insert the search field above the status bar.
		hl.root.RemoveItem(hl.statusBar)
		hl.root.AddItem(hl.searchBar, 1, 0, false)
		hl.root.AddItem(hl.statusBar, 2, 0, false)
	}
	hl.focusSearch()
}

// focusSearch moves focus to the search field.
func (hl *hostList) focusSearch() {
	hl.statusBar.SetText(searchStatusText())
	hl.app.tApp.SetFocus(hl.searchBar)
}

// focusResults moves focus to the filtered tree, keeping the filter active.
func (hl *hostList) focusResults() {
	hl.statusBar.SetText(statusText())
	hl.app.tApp.SetFocus(hl.tree)
}

func (hl *hostList) hideSearch() {
	if !hl.searching {
		return
	}
	hl.searching = false
	hl.filterText = ""
	hl.searchBar.SetText("")
	hl.root.RemoveItem(hl.searchBar)
	hl.rebuild()
	hl.statusBar.SetText(statusText())
	hl.app.tApp.SetFocus(hl.tree)
}

// --- actions ----------------------------------------------------------------

func (hl *hostList) confirmDelete(h *model.Host) {
	hl.app.showConfirm(fmt.Sprintf("Delete [yellow]%s[-]?", h.Name), func() {
		hl.deleteHost(h)
	})
}

func (hl *hostList) deleteHost(h *model.Host) {
	hosts := hl.app.store.Hosts[:0]
	for _, existing := range hl.app.store.Hosts {
		if existing.ID != h.ID {
			hosts = append(hosts, existing)
		}
	}
	hl.app.store.Hosts = hosts
	hl.app.save()
	hl.rebuild()
}

func (hl *hostList) doExport() {
	hl.app.promptInputModal("Export Hosts", "Export to file", config.Path(), func(path string) {
		if err := config.Export(path); err != nil {
			hl.app.showError("Export: " + err.Error())
		} else {
			hl.app.showError("Exported to " + path) // reuse showError for a simple info modal
		}
	})
}

func (hl *hostList) doImport() {
	hl.app.promptInputModal("Import Hosts", "Import from file", "", func(path string) {
		if err := config.Import(hl.app.store, path); err != nil {
			hl.app.showError("Import: " + err.Error())
		} else {
			hl.rebuild()
		}
	})
}

// --- helpers ----------------------------------------------------------------

func nodeHost(node *tview.TreeNode) (*model.Host, bool) {
	if node == nil {
		return nil, false
	}
	h, ok := node.GetReference().(*model.Host)
	return h, ok
}

func hostMatchesFilter(h *model.Host, lower string) bool {
	if strings.Contains(strings.ToLower(h.Name), lower) {
		return true
	}
	if strings.Contains(strings.ToLower(h.Hostname), lower) {
		return true
	}
	if strings.Contains(strings.ToLower(h.Folder), lower) {
		return true
	}
	for _, t := range h.Tags {
		if strings.Contains(strings.ToLower(t), lower) {
			return true
		}
	}
	return false
}

func searchStatusText() string {
	return "[yellow]↑↓[-] Move  [yellow]Enter/Tab[-] Go to results  [yellow]Esc[-] Clear search\n" +
		"In results: [yellow]Tab[-] Back to search  [yellow]Esc[-] Clear search"
}

func statusText() string {
	return "[yellow]Enter[-] Files  [yellow]s[-] SSH  [yellow]S[-] Plain SSH  [yellow]a[-] Add  [yellow]e[-] Edit  [yellow]d[-] Delete  [yellow]q[-] Quit\n" +
		"[yellow]/[-] Search  [yellow]I[-] Import  [yellow]E[-] Export"
}
