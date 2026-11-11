@echo off
del C:\td-capture\sink.stop 2>nul
C:\td-capture\sink.exe C:\td-capture\sink.log C:\td-capture\sink.stop
