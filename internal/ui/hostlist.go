package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/Bestigor89/lazyssh/internal/config"
	"github.com/Bestigor89/lazyssh/internal/model"
	sshpkg "github.com/Bestigor89/lazyssh/internal/ssh"
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
		hl.hideSearch()
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
}

// bindKeys attaches keyboard handlers to the tree widget.
func (hl *hostList) bindKeys() {
	hl.tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Typing '/' opens the search bar.
		if event.Key() == tcell.KeyRune && event.Rune() == '/' {
			hl.showSearch()
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
			case 's', 'S':
				// SSH terminal.
				if host != nil {
					go func() {
						if err := sshpkg.LaunchTerminal(hl.app.tApp, host); err != nil {
							hl.app.tApp.QueueUpdateDraw(func() {
								hl.app.showError(err.Error())
							})
						}
					}()
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
	if hl.searching {
		return
	}
	hl.searching = true
	// Replace status bar with search field.
	hl.root.RemoveItem(hl.statusBar)
	hl.root.AddItem(hl.searchBar, 1, 0, false)
	hl.app.tApp.SetFocus(hl.searchBar)
}

func (hl *hostList) hideSearch() {
	if !hl.searching {
		return
	}
	hl.searching = false
	hl.filterText = ""
	hl.searchBar.SetText("")
	hl.root.RemoveItem(hl.searchBar)
	hl.root.AddItem(hl.statusBar, 1, 0, false)
	hl.rebuild()
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

func statusText() string {
	return "[yellow]Enter[-] Files  [yellow]s[-] SSH  [yellow]a[-] Add  [yellow]e[-] Edit  [yellow]d[-] Delete  [yellow]q[-] Quit\n" +
		"[yellow]/[-] Search  [yellow]I[-] Import  [yellow]E[-] Export"
}
