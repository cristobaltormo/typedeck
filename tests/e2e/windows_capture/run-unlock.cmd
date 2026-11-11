@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File C:\td-capture\session-msg.ps1 -Code 8 > C:\td-capture\session-msg.txt 2>&1
