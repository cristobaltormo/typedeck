package engine

import (
	"errors"
	"log"

	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
)

func (e *Engine) handlers() capture.Handlers {
	return capture.Handlers{
		OnEvent: e.HandleEvent,
		Mask:    e.MaskBytes,
		OnConnected: func(i *capture.Info) {
			e.OnBoardConnected(i.Firmware)
			e.KeyboardChanged(i.Present)
		},
		OnDisconnect: e.OnBoardDisconnected,
	}
}

var ErrConsentRequired = errors.New("software capture needs explicit consent")

func (e *Engine) guardCapture(c config.Config) (config.Config, bool) {
	if c.Settings.Capture != capture.Software || config.HasCaptureConsent(e.paths) {
		return c, false
	}
	c.Settings.Capture = capture.Board
	log.Print("capture: the configuration asks for software capture but this installation never agreed to it; using the board")
	return c, true
}

func (e *Engine) applyCapture(kind string) {
	sw, ok := e.src.(capture.Switcher)
	if !ok || sw.Active() == kind {
		return
	}
	if err := sw.Select(kind); err != nil {
		log.Printf("capture mode %s: %v", kind, err)
		_ = sw.Select(capture.Board)
		go e.hudFn(e.tr("capture_failed"), e.tr("capture_failed_sub"), 6, true)
	}
}

func (e *Engine) CaptureMode() string {
	if sw, ok := e.src.(capture.Switcher); ok {
		return sw.Active()
	}
	return capture.Board
}

func (e *Engine) softwareMode() bool {
	return e.src != nil && e.src.Kind() == capture.Software
}

func (e *Engine) SetCapture(kind string, consent bool) error {
	sw, ok := e.src.(capture.Switcher)
	if !ok {
		return errors.New("this build cannot switch the capture mode")
	}
	if kind == capture.Software {
		if !consent {
			return ErrConsentRequired
		}
		if err := config.RecordCaptureConsent(e.paths); err != nil {
			return err
		}
	}
	prev := sw.Active()
	if err := sw.Select(kind); err != nil {
		_ = sw.Select(prev)
		return err
	}
	cfg := e.Config()
	cfg.Settings.Capture = kind
	if _, err := e.SetConfig(cfg); err != nil {
		_ = sw.Select(prev)
		return err
	}
	e.syncMaskAsync()
	e.syncKeysAsync()
	return nil
}
