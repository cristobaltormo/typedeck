#!/bin/sh
udevadm control --reload 2>/dev/null || true
udevadm trigger --subsystem-match=tty 2>/dev/null || true
exit 0
