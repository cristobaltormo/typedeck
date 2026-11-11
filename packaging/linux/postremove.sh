#!/bin/sh
udevadm control --reload 2>/dev/null || true
if [ ! -e /usr/lib/udev/rules.d/70-typedeck.rules ] && [ -e /dev/uinput ]; then
	chgrp root /dev/uinput 2>/dev/null || true
	chmod 0600 /dev/uinput 2>/dev/null || true
fi
exit 0
