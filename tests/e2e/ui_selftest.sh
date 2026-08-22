#!/bin/zsh
CH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
perl -e 'alarm 90; exec @ARGV' "$CH" --headless=new --disable-gpu --virtual-time-budget=40000 --dump-dom "http://127.0.0.1:7788/?selftest" 2>/dev/null | grep -o '<pre id="selftest">.*</pre>' | sed 's/<[^>]*>//g'
