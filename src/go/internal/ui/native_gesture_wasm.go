//go:build js && !test

package ui

// Called only when the set changed, so the clear+register churn never interrupts a steady gesture.
func platformSyncNativeRects(rects []NativeRect) {
	filePickerClearRects()
	softKeyboardClearRects()
	mobileInputClear()
	for _, r := range rects {
		switch r.Intent.Channel {
		case NativeFilePicker:
			filePickerRegisterRect(r.Intent.ID, r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(), r.Intent.Accept)
		case NativeSoftKeyboard:
			softKeyboardRegisterRect(r.Intent.ID, r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(), r.Intent.InputMode)
		case NativeTextInput:
			if r.Intent.InputRect.Empty() {
				mobileInputRegister(r.Intent.ID, r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(), r.Intent.Text, r.Intent.MaxLen, r.Intent.InputMode)
			} else {
				ir := r.Intent.InputRect
				mobileInputRegisterTrigger(r.Intent.ID,
					r.Rect.Min.X, r.Rect.Min.Y, r.Rect.Dx(), r.Rect.Dy(),
					ir.Min.X, ir.Min.Y, ir.Dx(), ir.Dy(),
					r.Intent.Text, r.Intent.MaxLen, r.Intent.InputMode)
			}
		}
	}
}
