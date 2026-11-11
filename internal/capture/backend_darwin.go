//go:build darwin

package capture

func newBackend() Backend { return NewMacBackend(helperPath) }
