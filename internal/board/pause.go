package board

import "strings"

func (b *Board) Pause() {
	if b.paused.Swap(true) {
		return
	}
	if b.write("MASK "+strings.Repeat("00", 32)) == nil {
		b.mu.Lock()
		b.lastMsk = strings.Repeat("00", 32)
		b.mu.Unlock()
	}
}

func (b *Board) Resume() {
	if !b.paused.Swap(false) {
		return
	}
	b.SyncMask(true)
}
