package main

import (
	"context"
	_ "embed"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"

	"fyne.io/systray"
	"golang.org/x/sys/windows"

	"github.com/mmcdole/kivo/internal/config"
)

// detached is true when the process has no stderr, as when kivow.exe, the
// build without a console, is started from the Start menu or at sign-in.
// It is read before anything replaces stderr.
var detached = func() bool {
	t, _ := windows.GetFileType(windows.Stderr)
	return t == windows.FILE_TYPE_UNKNOWN
}()

//go:embed kivo.ico
var trayIcon []byte

// inTray runs serve. Detached, it logs to logs\kivo.log in the data folder
// and shows an icon in the notification area to open Kivo, open the logs
// folder or quit. If a detached server is already running, it opens Kivo
// instead.
func inTray(ctx context.Context, boot config.Bootstrap, serve func(context.Context) error) error {
	if !detached {
		return serve(ctx)
	}
	url := localURL(boot)
	name, _ := windows.UTF16PtrFromString(`Local\Kivo`)
	if _, err := windows.CreateMutex(nil, false, name); errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		shellOpen(url)
		return nil
	}
	logDir := filepath.Join(boot.DataDir, "logs")
	if err := logToFile(logDir); err != nil {
		return err
	}

	ctx, stop := context.WithCancel(ctx)
	done, ready := make(chan error, 1), make(chan struct{})
	go func() {
		done <- serve(ctx)
		<-ready // Quit before the tray is up would be lost
		systray.Quit()
	}()
	var err error
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("Kivo")
		open := systray.AddMenuItem("Open Kivo", "")
		logs := systray.AddMenuItem("View logs", "")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "")
		close(ready)
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					shellOpen(url)
				case <-logs.ClickedCh:
					shellOpen(logDir)
				case <-quit.ClickedCh:
					systray.Quit()
				}
			}
		}()
	}, func() {
		// This runs in the tray's event loop, also when Windows signs out,
		// so the server stops before the process ends.
		stop()
		err = <-done
	})
	return err
}

// logToFile points stderr, and with it the log and any crash report, at
// kivo.log in dir. The previous run's log is kept as kivo.1.log.
func logToFile(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "kivo.log")
	_ = os.Rename(path, filepath.Join(dir, "kivo.1.log"))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd())); err != nil {
		return err
	}
	os.Stderr = f
	return nil
}

// localURL is the address of the web app on this machine.
func localURL(boot config.Bootstrap) string {
	host, port, _ := net.SplitHostPort(boot.Listen[0])
	if a, err := netip.ParseAddr(host); host == "" || err == nil && a.IsUnspecified() {
		host = "localhost"
	}
	scheme := "http"
	if boot.TLSCert != "" {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(host, port) + boot.BasePath + "/"
}

// shellOpen opens a URL in the default browser or a folder in Explorer.
func shellOpen(target string) {
	p, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return
	}
	_ = windows.ShellExecute(0, nil, p, nil, nil, windows.SW_SHOWNORMAL)
}

// alert shows err in a message box when there's no console to print it to.
func alert(err error) {
	if !detached {
		return
	}
	text, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString("Kivo")
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR)
}
