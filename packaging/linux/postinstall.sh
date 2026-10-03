#!/bin/sh
udevadm control --reload 2>/dev/null || true
udevadm trigger --action=add --subsystem-match=tty 2>/dev/null || true
exit 0
