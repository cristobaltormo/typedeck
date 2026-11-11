//go:build !windows && !linux && !darwin

package capture

import "github.com/cristobaltormo/typedeck/internal/hid"

type unsupported struct{}

func newBackend() Backend { return unsupported{} }

func (unsupported) Start(*Filter) error          { return ErrUnsupported }
func (unsupported) Alive() bool                  { return false }
func (unsupported) Stop()                        {}
func (unsupported) Inject(hid.Stroke, int) error { return ErrUnsupported }
func (unsupported) Consumer(uint16) error        { return ErrUnsupported }
func (unsupported) TypeText(string) error        { return ErrUnsupported }
func (unsupported) Latency() Latency             { return Latency{} }
func (unsupported) Checks() []Check {
	return []Check{{Problem: "software capture is not available on this system yet; use the board"}}
}
