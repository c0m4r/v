//go:build cgo
// +build cgo

package gui

import (
	"fmt"
	"strings"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func (app *App) showImageManager() {
	dlg, err := gtk.DialogNew()
	if err != nil {
		app.showError("Error", "Failed to create dialog: "+err.Error())
		return
	}
	defer dlg.Destroy()
	dlg.SetTitle("Image Manager")
	dlg.SetTransientFor(app.win)
	dlg.SetModal(true)
	_, _ = dlg.AddButton("Close", gtk.RESPONSE_CLOSE)
	dlg.SetDefaultSize(550, 400)

	content, err := dlg.GetContentArea()
	if err != nil {
		return
	}
	content.SetSpacing(8)
	content.SetMarginStart(16)
	content.SetMarginEnd(16)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)

	pullBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	if err != nil {
		return
	}
	content.PackStart(pullBox, false, false, 0)

	pullEntry, _ := gtk.EntryNew()
	pullEntry.SetPlaceholderText("Image name (e.g. ubuntu-24.04) or URL")
	pullEntry.SetHExpand(true)
	pullBox.PackStart(pullEntry, true, true, 0)

	cachedListBox, _ := gtk.ListBoxNew()

	var pullBtn *gtk.Button
	pullBtn, _ = gtk.ButtonNewWithLabel("Pull")
	pullBtn.Connect("clicked", func() {
		nameText, _ := pullEntry.GetText()
		name := strings.TrimSpace(nameText)
		if name == "" {
			return
		}
		pullBtn.SetSensitive(false)
		pullBtn.SetLabel("Downloading...")
		go func() {
			_, err := app.engine.PullImage(name, nil)
			glib.IdleAdd(func() {
				pullBtn.SetSensitive(true)
				pullBtn.SetLabel("Pull")
				if err != nil {
					app.showError("Pull Image", err.Error())
				} else {
					pullEntry.SetText("")
				}
				app.refreshImageList(cachedListBox)
				app.refreshVMList()
			})
		}()
	})
	pullBox.PackStart(pullBtn, false, false, 0)

	knownLabel, _ := gtk.LabelNew("<b>Known images (built-in shortcuts):</b>")
	knownLabel.SetUseMarkup(true)
	knownLabel.SetHAlign(gtk.ALIGN_START)
	knownLabel.SetMarginTop(8)
	content.PackStart(knownLabel, false, false, 0)

	knownImages := app.engine.KnownImages()
	knownText := ""
	for name, url := range knownImages {
		knownText += fmt.Sprintf("  %s  \u2192  %s\n", name, url)
	}
	knownScroll, _ := gtk.ScrolledWindowNew(nil, nil)
	knownScroll.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)
	knownScroll.SetMinContentHeight(80)
	knownBuf, _ := gtk.TextBufferNew(nil)
	knownBuf.SetText(knownText)
	knownView, _ := gtk.TextViewNewWithBuffer(knownBuf)
	knownView.SetEditable(false)
	knownView.SetCursorVisible(false)
	knownView.SetMonospace(true)
	knownScroll.Add(knownView)
	content.PackStart(knownScroll, false, true, 0)

	cachedLabel, _ := gtk.LabelNew("<b>Cached images:</b>")
	cachedLabel.SetUseMarkup(true)
	cachedLabel.SetHAlign(gtk.ALIGN_START)
	cachedLabel.SetMarginTop(8)
	content.PackStart(cachedLabel, false, false, 0)

	sw, _ := gtk.ScrolledWindowNew(nil, nil)
	sw.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)
	sw.SetMinContentHeight(150)
	content.PackStart(sw, true, true, 0)

	sw.Add(cachedListBox)

	app.refreshImageList(cachedListBox)

	dlg.ShowAll()
	dlg.Run()
}

func (app *App) refreshImageList(listBox *gtk.ListBox) {
	children := listBox.GetChildren()
	children.Foreach(func(item interface{}) {
		if row, ok := item.(*gtk.Widget); ok {
			listBox.Remove(row)
		}
	})

	images, err := app.engine.ListImages()
	if err != nil {
		label, _ := gtk.LabelNew("Error: " + err.Error())
		row, _ := gtk.ListBoxRowNew()
		row.Add(label)
		listBox.Insert(row, -1)
		listBox.ShowAll()
		return
	}

	if len(images) == 0 {
		label, _ := gtk.LabelNew("No cached images. Pull one above.")
		row, _ := gtk.ListBoxRowNew()
		row.Add(label)
		row.SetSensitive(false)
		listBox.Insert(row, -1)
		listBox.ShowAll()
		return
	}

	for _, img := range images {
		row, _ := gtk.ListBoxRowNew()
		box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 12)
		box.SetMarginStart(8)
		box.SetMarginEnd(8)
		box.SetMarginTop(4)
		box.SetMarginBottom(4)
		row.Add(box)

		nameLabel, _ := gtk.LabelNew(img.Name)
		nameLabel.SetHAlign(gtk.ALIGN_START)
		nameLabel.SetHExpand(true)
		box.PackStart(nameLabel, true, true, 0)

		sizeStr := formatBytes(img.Size)
		sizeLabel, _ := gtk.LabelNew(sizeStr)
		sizeLabel.SetHAlign(gtk.ALIGN_END)
		box.PackEnd(sizeLabel, false, false, 0)

		listBox.Insert(row, -1)
	}

	listBox.ShowAll()
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GiB", float64(n)/(1024*1024*1024))
}
