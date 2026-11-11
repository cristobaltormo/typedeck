@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File C:\td-capture\session-msg.ps1 -Code 7 > C:\td-capture\session-msg.txt 2>&1
