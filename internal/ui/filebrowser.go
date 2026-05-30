package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/Bestigor89/lazyssh/internal/model"
	sshpkg "github.com/Bestigor89/lazyssh/internal/ssh"
)

const (
	panelLocal  = 0
	panelRemote = 1
)

// fileBrowser is the dual-pane file transfer screen.
type fileBrowser struct {
	root        *tview.Flex
	localTable  *tview.Table
	remoteTable *tview.Table
	localPath   *tview.TextView
	remotePath  *tview.TextView
	statusBar   *tview.TextView

	app    *App
	host   *model.Host
	client *sshpkg.Client

	localDir      string
	remoteDir     string
	active        int // panelLocal or panelRemote
	remoteLoading bool

	localFiles  []localEntry
	remoteFiles []sshpkg.FileInfo

	localPathStr  string // stored to re-render path bars on panel switch
	remotePathStr string
}

type localEntry struct {
	name    string
	isDir   bool
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func newFileBrowser(a *App, host *model.Host, client *sshpkg.Client) *fileBrowser {
	fb := &fileBrowser{
		app:    a,
		host:   host,
		client: client,
		active: panelLocal,
	}

	fb.localDir, _ = os.Getwd()
	fb.remoteDir = client.HomeDir()
	fb.localPathStr = fb.localDir
	fb.remotePathStr = fb.remoteDir

	fb.localPath = tview.NewTextView().SetDynamicColors(true)
	fb.remotePath = tview.NewTextView().SetDynamicColors(true)
	fb.localTable = makeTable()
	fb.remoteTable = makeTable()

	fb.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText(browserStatusText())

	localPane := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(fb.localPath, 1, 0, false).
		AddItem(fb.localTable, 0, 1, true)

	remotePane := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(fb.remotePath, 1, 0, false).
		AddItem(fb.remoteTable, 0, 1, true)

	divider := tview.NewTextView().SetText("│")

	panelRow := tview.NewFlex().
		AddItem(localPane, 0, 1, true).
		AddItem(divider, 1, 0, false).
		AddItem(remotePane, 0, 1, false)

	titleBar := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf("[::b]Files[-]  [gray]%s  %s[-]", host.Name, host.UserHost()))

	fb.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(titleBar, 1, 0, false).
		AddItem(panelRow, 0, 1, true).
		AddItem(fb.statusBar, 2, 0, false)

	fb.loadLocal()
	fb.startRemoteLoad()
	fb.bindKeys()
	fb.highlightActive()
	return fb
}

func makeTable() *tview.Table {
	return tview.NewTable().
		SetSelectable(true, false).
		SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorNavy).Foreground(tcell.ColorWhite))
}

// --- loading ----------------------------------------------------------------

func (fb *fileBrowser) loadLocal() {
	entries, err := readLocalDir(fb.localDir)
	if err != nil {
		fb.app.showError("Local dir: " + err.Error())
		return
	}
	fb.localFiles = entries
	fb.renderTable(fb.localTable, localEntriesToDisplay(entries))
	fb.localPathStr = fb.localDir
	fb.refreshPathBars()
}

func (fb *fileBrowser) startRemoteLoad() {
	if fb.remoteLoading {
		return
	}
	fb.remoteLoading = true
	fb.remoteTable.Clear()
	fb.remoteTable.SetCell(0, 0,
		tview.NewTableCell("[gray]Loading…[-]").SetExpansion(1).SetSelectable(false))
	fb.remotePath.SetText(fmt.Sprintf("  [gray]Remote: %s  Loading…[-]", fb.remoteDir))

	dir := fb.remoteDir
	go func() {
		entries, err := fb.client.ListDir(dir)
		fb.app.tApp.QueueUpdateDraw(func() {
			fb.remoteLoading = false
			if err != nil {
				fb.app.showError("Remote dir: " + err.Error())
				fb.remoteTable.Clear()
				return
			}
			fb.remoteFiles = entries
			fb.renderTable(fb.remoteTable, remoteEntriesToDisplay(entries))
			fb.remotePathStr = dir
			fb.refreshPathBars()
		})
	}()
}

// renderTable fills a table with ".." + file rows (4 columns).
func (fb *fileBrowser) renderTable(t *tview.Table, rows []tableRow) {
	t.Clear()
	t.SetCell(0, 0, tview.NewTableCell("[gray]..[-]").SetExpansion(1))
	t.SetCell(0, 1, tview.NewTableCell(""))
	t.SetCell(0, 2, tview.NewTableCell(""))
	t.SetCell(0, 3, tview.NewTableCell(""))
	for i, r := range rows {
		t.SetCell(i+1, 0, tview.NewTableCell(r.name).SetExpansion(1))
		t.SetCell(i+1, 1, tview.NewTableCell("[gray]"+r.perms+"[-]").SetMaxWidth(11))
		t.SetCell(i+1, 2, tview.NewTableCell(r.size).SetAlign(tview.AlignRight).SetMaxWidth(8))
		t.SetCell(i+1, 3, tview.NewTableCell("[gray]"+r.date+"[-]").SetMaxWidth(13))
	}
	t.Select(0, 0)
}

type tableRow struct {
	name  string
	perms string
	size  string
	date  string
	isDir bool
}

func localEntriesToDisplay(entries []localEntry) []tableRow {
	rows := make([]tableRow, len(entries))
	for i, e := range entries {
		rows[i] = tableRow{
			name:  formatEntryName(e.name, e.isDir),
			perms: formatPerms(e.mode),
			size:  formatSize(e.size, e.isDir),
			date:  formatDate(e.modTime),
			isDir: e.isDir,
		}
	}
	return rows
}

func remoteEntriesToDisplay(entries []sshpkg.FileInfo) []tableRow {
	rows := make([]tableRow, len(entries))
	for i, e := range entries {
		rows[i] = tableRow{
			name:  formatEntryName(e.Name, e.IsDir),
			perms: formatPerms(e.Mode),
			size:  formatSize(e.Size, e.IsDir),
			date:  formatDate(e.ModTime),
			isDir: e.IsDir,
		}
	}
	return rows
}

func formatEntryName(name string, isDir bool) string {
	if isDir {
		return "[blue]▶ " + name + "/[-]"
	}
	return "  " + name
}

func formatSize(size int64, isDir bool) string {
	if isDir {
		return ""
	}
	const k = 1024
	switch {
	case size < k:
		return fmt.Sprintf("%d B", size)
	case size < k*k:
		return fmt.Sprintf("%.1fK", float64(size)/k)
	case size < k*k*k:
		return fmt.Sprintf("%.1fM", float64(size)/(k*k))
	default:
		return fmt.Sprintf("%.1fG", float64(size)/(k*k*k))
	}
}

func formatPerms(mode os.FileMode) string {
	s := mode.String()
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.Year() == time.Now().Year() {
		return t.Format("Jan 02 15:04")
	}
	return t.Format("Jan 02  2006")
}

// --- panel highlight --------------------------------------------------------

// refreshPathBars re-renders both path-bar labels with active/inactive styling.
func (fb *fileBrowser) refreshPathBars() {
	activeSelectStyle := tcell.StyleDefault.
		Background(tcell.ColorNavy).Foreground(tcell.ColorWhite)
	inactiveSelectStyle := tcell.StyleDefault.
		Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorSilver)

	if fb.active == panelLocal {
		fb.localTable.SetSelectedStyle(activeSelectStyle)
		fb.remoteTable.SetSelectedStyle(inactiveSelectStyle)
		fb.localPath.SetBackgroundColor(tcell.ColorDarkBlue)
		fb.localPath.SetText(fmt.Sprintf("[yellow::b] ▶ Local:[-::-] [white]%s[-]", fb.localPathStr))
		fb.remotePath.SetBackgroundColor(tcell.ColorDefault)
		fb.remotePath.SetText(fmt.Sprintf("[gray]   Remote: %s[-]", fb.remotePathStr))
	} else {
		fb.localTable.SetSelectedStyle(inactiveSelectStyle)
		fb.remoteTable.SetSelectedStyle(activeSelectStyle)
		fb.localPath.SetBackgroundColor(tcell.ColorDefault)
		fb.localPath.SetText(fmt.Sprintf("[gray]   Local: %s[-]", fb.localPathStr))
		fb.remotePath.SetBackgroundColor(tcell.ColorDarkGreen)
		fb.remotePath.SetText(fmt.Sprintf("[green::b] ▶ Remote:[-::-] [white]%s[-]", fb.remotePathStr))
	}
}

func (fb *fileBrowser) highlightActive() {
	fb.refreshPathBars()
}

func (fb *fileBrowser) focusActive() {
	if fb.active == panelLocal {
		fb.app.tApp.SetFocus(fb.localTable)
	} else {
		fb.app.tApp.SetFocus(fb.remoteTable)
	}
}

func (fb *fileBrowser) activeTable() *tview.Table {
	if fb.active == panelLocal {
		return fb.localTable
	}
	return fb.remoteTable
}

// --- key bindings -----------------------------------------------------------

func (fb *fileBrowser) bindKeys() {
	capture := func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			fb.switchPanel()
			return nil
		}
		if event.Key() == tcell.KeyEscape {
			fb.client.Close()
			fb.app.closeFileBrowser()
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			fb.openSelected()
			return nil
		}

		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'u', 'U':
				fb.upload()
				return nil
			case 'd', 'D':
				fb.download()
				return nil
			case 'n', 'N':
				fb.createFile()
				return nil
			case 'm', 'M':
				fb.mkdir()
				return nil
			case 'x', 'X':
				fb.deleteSelected()
				return nil
			case 'v', 'V':
				fb.viewFile()
				return nil
			case 'e', 'E':
				fb.editFile()
				return nil
			case 't', 'T':
				fb.openTerminal()
				return nil
			}
		}

		switch event.Key() {
		case tcell.KeyF3:
			fb.viewFile()
			return nil
		case tcell.KeyF4:
			fb.editFile()
			return nil
		case tcell.KeyF5:
			fb.upload()
			return nil
		case tcell.KeyF6:
			fb.download()
			return nil
		case tcell.KeyF7:
			fb.mkdir()
			return nil
		case tcell.KeyF8, tcell.KeyDelete:
			fb.deleteSelected()
			return nil
		case tcell.KeyCtrlO:
			fb.openTerminal()
			return nil
		}
		return event
	}

	fb.localTable.SetInputCapture(capture)
	fb.remoteTable.SetInputCapture(capture)
}

func (fb *fileBrowser) switchPanel() {
	if fb.active == panelLocal {
		fb.active = panelRemote
		fb.app.tApp.SetFocus(fb.remoteTable)
	} else {
		fb.active = panelLocal
		fb.app.tApp.SetFocus(fb.localTable)
	}
	fb.refreshPathBars()
}

// --- navigation -------------------------------------------------------------

func (fb *fileBrowser) openSelected() {
	row, _ := fb.activeTable().GetSelection()
	if fb.active == panelLocal {
		fb.openLocalRow(row)
	} else {
		if fb.remoteLoading {
			return
		}
		fb.openRemoteRow(row)
	}
}

func (fb *fileBrowser) openLocalRow(row int) {
	if row == 0 {
		fb.localDir = filepath.Dir(fb.localDir)
		fb.loadLocal()
		return
	}
	idx := row - 1
	if idx >= len(fb.localFiles) {
		return
	}
	if fb.localFiles[idx].isDir {
		fb.localDir = filepath.Join(fb.localDir, fb.localFiles[idx].name)
		fb.loadLocal()
	}
}

func (fb *fileBrowser) openRemoteRow(row int) {
	if row == 0 {
		fb.remoteDir = remoteParent(fb.remoteDir)
		fb.startRemoteLoad()
		return
	}
	idx := row - 1
	if idx >= len(fb.remoteFiles) {
		return
	}
	if fb.remoteFiles[idx].IsDir {
		fb.remoteDir = strings.TrimRight(fb.remoteDir, "/") + "/" + fb.remoteFiles[idx].Name
		fb.startRemoteLoad()
	}
}

func remoteParent(path string) string {
	path = strings.TrimRight(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return "/"
	}
	return path[:idx]
}

// --- upload / download (files + recursive dirs) ----------------------------

func (fb *fileBrowser) upload() {
	row, _ := fb.localTable.GetSelection()
	if row == 0 {
		fb.app.showError("Select a file or directory in the left panel first")
		return
	}
	idx := row - 1
	if idx >= len(fb.localFiles) {
		return
	}
	e := fb.localFiles[idx]
	localPath := filepath.Join(fb.localDir, e.name)
	remotePath := strings.TrimRight(fb.remoteDir, "/") + "/" + e.name

	if e.isDir {
		label := e.name + "/"
		updateFn, closeFn, ctx := fb.app.showUpdatableInfo("Uploading [yellow]" + label + "[-]…")
		count := 0
		go func() {
			err := fb.client.UploadDir(ctx, localPath, remotePath, func(rel string) {
				count++
				updateFn(fmt.Sprintf("Uploading [yellow]%s[-]\n[gray]%s[-]  (%d files)", label, rel, count))
			})
			cancelled := ctx.Err() != nil
			closeFn()
			fb.app.tApp.QueueUpdateDraw(func() {
				if cancelled {
					fb.focusActive()
					return // cancelled
				}
				if err != nil {
					fb.app.showError("Upload dir: " + err.Error())
				} else {
					fb.startRemoteLoad()
				}
			})
		}()
	} else {
		updateFn, closeFn, ctx := fb.app.showUpdatableInfo(fmt.Sprintf("Uploading [yellow]%s[-]…", e.name))
		go func() {
			err := fb.client.Upload(localPath, remotePath, func(n int64) {
				updateFn(fmt.Sprintf("Uploading [yellow]%s[-]\n%s", e.name, formatSize(n, false)))
			})
			cancelled := ctx.Err() != nil
			closeFn()
			fb.app.tApp.QueueUpdateDraw(func() {
				if cancelled {
					fb.focusActive()
					return
				}
				if err != nil {
					fb.app.showError("Upload: " + err.Error())
				} else {
					fb.startRemoteLoad()
				}
			})
		}()
	}
}

func (fb *fileBrowser) download() {
	if fb.remoteLoading {
		fb.app.showError("Remote panel is loading, please wait")
		return
	}
	row, _ := fb.remoteTable.GetSelection()
	if row == 0 {
		fb.app.showError("Select a file or directory in the right panel first")
		return
	}
	idx := row - 1
	if idx >= len(fb.remoteFiles) {
		return
	}
	e := fb.remoteFiles[idx]
	remotePath := strings.TrimRight(fb.remoteDir, "/") + "/" + e.Name
	localPath := filepath.Join(fb.localDir, e.Name)

	if e.IsDir {
		label := e.Name + "/"
		updateFn, closeFn, ctx := fb.app.showUpdatableInfo("Downloading [yellow]" + label + "[-]…")
		count := 0
		go func() {
			err := fb.client.DownloadDir(ctx, remotePath, localPath, func(name string) {
				count++
				updateFn(fmt.Sprintf("Downloading [yellow]%s[-]\n[gray]%s[-]  (%d files)", label, name, count))
			})
			cancelled := ctx.Err() != nil
			closeFn()
			fb.app.tApp.QueueUpdateDraw(func() {
				if cancelled {
					fb.focusActive()
					return // cancelled
				}
				if err != nil {
					fb.app.showError("Download dir: " + err.Error())
				} else {
					fb.loadLocal()
					fb.focusActive()
				}
			})
		}()
	} else {
		updateFn, closeFn, ctx := fb.app.showUpdatableInfo(fmt.Sprintf("Downloading [yellow]%s[-]…", e.Name))
		go func() {
			err := fb.client.Download(remotePath, localPath, func(n int64) {
				updateFn(fmt.Sprintf("Downloading [yellow]%s[-]\n%s", e.Name, formatSize(n, false)))
			})
			cancelled := ctx.Err() != nil
			closeFn()
			fb.app.tApp.QueueUpdateDraw(func() {
				if cancelled {
					fb.focusActive()
					return
				}
				if err != nil {
					fb.app.showError("Download: " + err.Error())
				} else {
					fb.loadLocal()
					fb.focusActive()
				}
			})
		}()
	}
}

// --- create file (n) --------------------------------------------------------

func (fb *fileBrowser) createFile() {
	if fb.active == panelLocal {
		fb.app.promptInputModal("New Local File", "Filename", "", func(name string) {
			fb.focusActive()
			if name == "" {
				return
			}
			f, err := os.Create(filepath.Join(fb.localDir, name))
			if err != nil {
				fb.app.showError("Create: " + err.Error())
				return
			}
			f.Close()
			fb.loadLocal()
		})
	} else {
		if fb.remoteLoading {
			return
		}
		fb.app.promptInputModal("New Remote File", "Filename", "", func(name string) {
			fb.focusActive()
			if name == "" {
				return
			}
			path := strings.TrimRight(fb.remoteDir, "/") + "/" + name
			if err := fb.client.CreateRemoteFile(path); err != nil {
				fb.app.showError("Create remote: " + err.Error())
			} else {
				fb.startRemoteLoad()
			}
		})
	}
}

// --- mkdir / delete ---------------------------------------------------------

func (fb *fileBrowser) mkdir() {
	if fb.active == panelLocal {
		fb.app.promptInputModal("Create Local Directory", "Name", "", func(name string) {
			fb.focusActive()
			if name == "" {
				return
			}
			if err := os.Mkdir(filepath.Join(fb.localDir, name), 0o755); err != nil {
				fb.app.showError("Mkdir: " + err.Error())
			} else {
				fb.loadLocal()
			}
		})
	} else {
		if fb.remoteLoading {
			return
		}
		fb.app.promptInputModal("Create Remote Directory", "Name", "", func(name string) {
			fb.focusActive()
			if name == "" {
				return
			}
			if err := fb.client.MkdirRemote(strings.TrimRight(fb.remoteDir, "/") + "/" + name); err != nil {
				fb.app.showError("Mkdir remote: " + err.Error())
			} else {
				fb.startRemoteLoad()
			}
		})
	}
}

func (fb *fileBrowser) deleteSelected() {
	row, _ := fb.activeTable().GetSelection()
	if row == 0 {
		return
	}

	if fb.active == panelLocal {
		idx := row - 1
		if idx >= len(fb.localFiles) {
			return
		}
		e := fb.localFiles[idx]

		msg := fmt.Sprintf("Delete local [yellow]%s[-]?", e.name)
		if e.isDir {
			msg = fmt.Sprintf("Delete local directory [yellow]%s/[-] and ALL its contents?", e.name)
		}
		fb.app.showConfirm(msg, func() {
			path := filepath.Join(fb.localDir, e.name)
			var err error
			if e.isDir {
				err = os.RemoveAll(path)
			} else {
				err = os.Remove(path)
			}
			if err != nil {
				fb.app.showError("Delete: " + err.Error())
			} else {
				fb.loadLocal()
			}
			fb.focusActive()
		})
	} else {
		if fb.remoteLoading {
			return
		}
		idx := row - 1
		if idx >= len(fb.remoteFiles) {
			return
		}
		e := fb.remoteFiles[idx]
		remotePath := strings.TrimRight(fb.remoteDir, "/") + "/" + e.Name

		msg := fmt.Sprintf("Delete remote [yellow]%s[-]?", e.Name)
		if e.IsDir {
			msg = fmt.Sprintf("Delete remote directory [yellow]%s/[-] and ALL its contents?", e.Name)
		}
		fb.app.showConfirm(msg, func() {
			if !e.IsDir {
				// Simple file removal — fast, no goroutine needed.
				if err := fb.client.RemoveRemote(remotePath); err != nil {
					fb.app.showError("Delete: " + err.Error())
				} else {
					fb.startRemoteLoad()
				}
				fb.focusActive()
				return
			}
			// Directory: recursive removal — runs in background.
			fb.app.showInfo("Deleting " + e.Name + "/…")
			go func() {
				err := fb.client.RemoveRemoteAll(remotePath)
				fb.app.tApp.QueueUpdateDraw(func() {
					fb.app.removeModal()
					if err != nil {
						fb.app.showError("Delete: " + err.Error())
					} else {
						fb.startRemoteLoad()
					}
					fb.focusActive()
				})
			}()
		})
	}
}

// --- viewer (F3 / v) --------------------------------------------------------

func (fb *fileBrowser) viewFile() {
	row, _ := fb.activeTable().GetSelection()
	if row == 0 {
		return
	}
	if fb.active == panelLocal {
		idx := row - 1
		if idx >= len(fb.localFiles) || fb.localFiles[idx].isDir {
			return
		}
		fb.viewLocalFile(fb.localFiles[idx].name)
	} else {
		if fb.remoteLoading {
			return
		}
		idx := row - 1
		if idx >= len(fb.remoteFiles) || fb.remoteFiles[idx].IsDir {
			return
		}
		fb.viewRemoteFile(fb.remoteFiles[idx].Name)
	}
}

func (fb *fileBrowser) viewLocalFile(name string) {
	path := filepath.Join(fb.localDir, name)
	const limit = 512 * 1024
	data, err := os.ReadFile(path)
	if err != nil {
		fb.app.showError("Read: " + err.Error())
		return
	}
	truncated := false
	if int64(len(data)) > limit {
		data = data[:limit]
		truncated = true
	}
	fb.showViewer(path, data, truncated)
}

func (fb *fileBrowser) viewRemoteFile(name string) {
	remotePath := strings.TrimRight(fb.remoteDir, "/") + "/" + name
	fb.app.showInfo("Loading " + name + "…")
	const limit = 512 * 1024
	go func() {
		data, err := fb.client.ReadRemoteFile(remotePath, limit+1)
		fb.app.tApp.QueueUpdateDraw(func() {
			fb.app.removeModal()
			if err != nil {
				fb.app.showError("Read remote: " + err.Error())
				return
			}
			truncated := false
			if int64(len(data)) > limit {
				data = data[:limit]
				truncated = true
			}
			fb.showViewer(remotePath, data, truncated)
		})
	}()
}

func (fb *fileBrowser) showViewer(path string, data []byte, truncated bool) {
	text := string(data)
	if truncated {
		text += "\n\n── file truncated at 512 KB ──"
	}

	tv := tview.NewTextView().
		SetScrollable(true).
		SetWrap(false).
		SetDynamicColors(false).
		SetText(text)

	titleBar := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[yellow]View:[-] %s", path))

	hint := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow]↑↓/PgUp/PgDn[-] Scroll  [yellow]←→[-] Horizontal  [yellow]Esc[-] Close")

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(titleBar, 1, 0, false).
		AddItem(tv, 0, 1, true).
		AddItem(hint, 1, 0, false)

	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			fb.app.pages.RemovePage("viewer")
			fb.app.pages.SwitchToPage("browser")
			fb.focusActive()
			return nil
		}
		return event
	})

	fb.app.pages.AddPage("viewer", page, true, true)
	fb.app.pages.SwitchToPage("viewer")
	fb.app.tApp.SetFocus(tv)
}

// --- editor (F4 / e) --------------------------------------------------------

func (fb *fileBrowser) editFile() {
	row, _ := fb.activeTable().GetSelection()
	if row == 0 {
		return
	}
	if fb.active == panelLocal {
		idx := row - 1
		if idx >= len(fb.localFiles) || fb.localFiles[idx].isDir {
			return
		}
		fb.editLocalFile(fb.localFiles[idx].name)
	} else {
		if fb.remoteLoading {
			return
		}
		idx := row - 1
		if idx >= len(fb.remoteFiles) || fb.remoteFiles[idx].IsDir {
			return
		}
		fb.editRemoteFile(fb.remoteFiles[idx].Name)
	}
}

func (fb *fileBrowser) editLocalFile(name string) {
	path := filepath.Join(fb.localDir, name)
	// Called from SetInputCapture = main (UI) goroutine.
	// Calling openEditor directly (no goroutine) guarantees correct terminal state.
	openEditor(fb.app.tApp, path)
	fb.loadLocal()
}

func (fb *fileBrowser) editRemoteFile(name string) {
	remotePath := strings.TrimRight(fb.remoteDir, "/") + "/" + name

	tmp, err := os.CreateTemp("", "lazyssh-edit-*")
	if err != nil {
		fb.app.showError("Temp file: " + err.Error())
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()

	fb.app.showInfo("Downloading " + name + " for editing…")

	go func() {
		if err := fb.client.Download(remotePath, tmpPath, nil); err != nil {
			os.Remove(tmpPath)
			fb.app.tApp.QueueUpdateDraw(func() {
				fb.app.removeModal()
				fb.app.showError("Download for edit: " + err.Error())
			})
			return
		}

		stat1, _ := os.Stat(tmpPath)

		// Open editor from the main goroutine via QueueUpdateDraw.
		// QueueUpdateDraw runs in the event loop — Suspend from here gives
		// the editor exclusive terminal control without any race condition.
		fb.app.tApp.QueueUpdateDraw(func() {
			fb.app.removeModal()
			openEditor(fb.app.tApp, tmpPath) // blocks until editor exits

			stat2, _ := os.Stat(tmpPath)
			if stat1 != nil && stat2 != nil && stat1.ModTime().Equal(stat2.ModTime()) {
				os.Remove(tmpPath)
				return // not modified
			}

			fb.app.showInfo("Uploading " + name + "…")
			go func() {
				defer os.Remove(tmpPath)
				if err := fb.client.Upload(tmpPath, remotePath, nil); err != nil {
					fb.app.tApp.QueueUpdateDraw(func() {
						fb.app.removeModal()
						fb.app.showError("Upload after edit: " + err.Error())
					})
					return
				}
				fb.app.tApp.QueueUpdateDraw(func() {
					fb.app.removeModal()
					fb.startRemoteLoad()
				})
			}()
		})
	}()
}

// openEditor suspends the TUI, resets the terminal to sane defaults (so that
// TUI editors like nano/vim receive all keystrokes), then opens $EDITOR.
func openEditor(tApp *tview.Application, path string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, e := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return
	}

	tApp.Suspend(func() {
		// stty sane restores the terminal to well-known defaults.
		// tcell's Suspend may leave some settings that confuse nano/vim.
		if sttyPath, err := exec.LookPath("stty"); err == nil {
			c := exec.Command(sttyPath, "sane")
			c.Stdin = os.Stdin
			_ = c.Run()
		}

		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	})
}

// --- SSH terminal (t / Ctrl+O) ----------------------------------------------

func (fb *fileBrowser) openTerminal() {
	go func() {
		if err := sshpkg.LaunchTerminal(fb.app.tApp, fb.host); err != nil {
			fb.app.tApp.QueueUpdateDraw(func() {
				fb.app.showError(err.Error())
			})
		}
	}()
}

// --- local FS helpers -------------------------------------------------------

func readLocalDir(path string) ([]localEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs, files []localEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fi := localEntry{
			name:    e.Name(),
			isDir:   e.IsDir(),
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
		}
		if e.IsDir() {
			dirs = append(dirs, fi)
		} else {
			files = append(files, fi)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return append(dirs, files...), nil
}

func browserStatusText() string {
	return "[yellow]Tab[-]Switch  [yellow]Enter[-]Open  [yellow]u[-]/F5 ↑Upload  [yellow]d[-]/F6 ↓Download  " +
		"[yellow]v[-]/F3 View  [yellow]e[-]/F4 Edit  [yellow]n[-] New file\n" +
		"[yellow]t[-]/^O SSH  [yellow]m[-]/F7 Mkdir  [yellow]x[-]/F8 Delete  [yellow]Esc[-] Back"
}
