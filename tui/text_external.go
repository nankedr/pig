package tui

import (
	"context"
	"errors"
)

// EditExternally lends the terminal to edit and replaces the expanded draft only on success.
// Rendering is paused while Session state can continue to change. Stop cancels and waits for edit.
func (u *TextUI) EditExternally(ctx context.Context, edit func(context.Context, string) (string, error)) (err error) {
	if ctx == nil || edit == nil {
		return errors.New("external editor requires a context and callback")
	}
	u.mu.Lock()
	if !u.started || u.stopped || u.paused {
		u.mu.Unlock()
		return errors.New("terminal is unavailable for external editor")
	}
	content, err := u.editor.GetExpandedText()
	if err != nil {
		u.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	u.externalCancel, u.externalDone = cancel, done
	u.paused = true
	u.generation++
	u.editor.cancelAutocomplete()
	mode, state := u.mode, u.mainState
	u.mu.Unlock()
	defer func() {
		cancel()
		u.mu.Lock()
		defer u.mu.Unlock()
		defer close(done)
		u.externalCancel, u.externalDone = nil, nil
		u.paused = false
		if u.stopped {
			u.started = false
			return
		}
		startErr := u.terminal.Start(u.input, func() { _ = u.Refresh() })
		if startErr == nil && u.mode == TUIModeFullscreen {
			startErr = u.terminal.Write(enterAltScreen + enableMouse())
		}
		if startErr != nil {
			u.finish(startErr)
			err = errors.Join(err, startErr)
			return
		}
		u.mainState = TUIMainScreenRenderState{PreviousWidth: -1, PreviousHeight: -1}
		u.altState = TUIMainScreenRenderState{}
		u.mouse = scrollMouse{}
		u.editor.Focused = true
		err = errors.Join(err, u.render())
		u.watchTerminalLocked()
	}()
	if mode == TUIModeFullscreen {
		err = u.terminal.Write(leaveAltScreen)
	} else {
		err = u.terminal.Write(mainScreenStop(state))
	}
	err = errors.Join(err, u.terminal.Stop())
	if source, ok := u.terminal.(interface{ Done() <-chan struct{} }); ok {
		<-source.Done()
	}
	if err != nil {
		return err
	}
	text, err := edit(ctx, content)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		u.mu.Lock()
		err = u.editor.SetText(text)
		u.mu.Unlock()
	}
	return err
}
