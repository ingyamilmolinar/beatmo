//go:build fyne

package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// RunFynePanel launches a control window implemented with Fyne.
func RunFynePanel(g *Game) {
	go func() {
		a := app.New()
		w := a.NewWindow("Controls")

		playBtn := widget.NewButton("Play", func() { g.drum.playPressed = true })
		stopBtn := widget.NewButton("Stop", func() { g.drum.stopPressed = true })

		bpmEntry := widget.NewEntry()
		bpmEntry.SetText(strconv.Itoa(g.drum.BPM()))
		bpmEntry.OnSubmitted = func(s string) {
			if v, err := strconv.Atoi(s); err == nil {
				g.drum.SetBPM(v)
			}
		}

		lenEntry := widget.NewEntry()
		lenEntry.SetText(strconv.Itoa(g.drum.Length))
		lenEntry.OnSubmitted = func(s string) {
			if v, err := strconv.Atoi(s); err == nil {
				g.drum.SetLength(v)
			}
		}

		instSelect := widget.NewSelect(audio.Instruments(), func(id string) {
			g.drum.SetInstrument(id)
		})
		if opts := audio.Instruments(); len(opts) > 0 {
			instSelect.SetSelected(opts[0])
		}

		uploadBtn := widget.NewButton("Upload WAV", func() {
			fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
				if err != nil || r == nil {
					return
				}
				path := r.URI().Path()
				r.Close()
				nameDlg := dialog.NewEntryDialog("Instrument Name", "Short name", func(name string) {
					if name == "" {
						return
					}
					if err := audio.RegisterWAV(name, path); err == nil {
						g.drum.AddInstrument(name)
						instSelect.Options = audio.Instruments()
						instSelect.SetSelected(name)
					}
				}, w)
				nameDlg.Show()
			}, w)
			fd.SetFilter(storage.NewExtensionFileFilter([]string{".wav"}))
			fd.Show()
		})

		w.SetContent(container.NewVBox(playBtn, stopBtn, bpmEntry, lenEntry, instSelect, uploadBtn))
		w.ShowAndRun()
	}()
}
