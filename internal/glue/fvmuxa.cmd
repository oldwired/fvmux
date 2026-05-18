@echo off
REM fvmuxa.cmd -- run fvmux inside a private tmux session on Windows.
REM
REM Usage:
REM   fvmuxa                    start or attach
REM   fvmuxa -session=NAME      flags pass through to fvmux
REM
REM Detach: F12 d   (tmux prefix is F12 in fvmux.tmux.conf)
REM Reattach: run fvmuxa again
REM Kill the host: tmux -L fvmux kill-server   (very rare)
REM
REM Requires tmux.exe and fvmux.exe on PATH. The tmux server uses a
REM private socket named "fvmux" (-L fvmux), so this does not interact
REM with any other tmux sessions the user may have.

setlocal

where tmux >nul 2>&1
if errorlevel 1 (
    echo fvmuxa: tmux not found on PATH. 1>&2
    exit /b 1
)
where fvmux >nul 2>&1
if errorlevel 1 (
    echo fvmuxa: fvmux not found on PATH. 1>&2
    exit /b 1
)

REM Honour FVMUX_TMUX_CONF if the user set it explicitly; otherwise
REM use the default install location, then fall back to a config next
REM to this script (handy when running uninstalled out of the repo).
if not defined FVMUX_TMUX_CONF (
    set "FVMUX_TMUX_CONF=%USERPROFILE%\.config\fvmux\fvmux.tmux.conf"
)
if not exist "%FVMUX_TMUX_CONF%" (
    if exist "%~dp0fvmux.tmux.conf" set "FVMUX_TMUX_CONF=%~dp0fvmux.tmux.conf"
)
if not exist "%FVMUX_TMUX_CONF%" (
    echo fvmuxa: tmux config not found at "%FVMUX_TMUX_CONF%". 1>&2
    echo Run fvmux's first-run wizard (Help ^> Reset First-Run^) to install. 1>&2
    exit /b 1
)

tmux -L fvmux -f "%FVMUX_TMUX_CONF%" new-session -A -s fvmux -- fvmux %*

endlocal
