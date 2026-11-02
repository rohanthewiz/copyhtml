//go:build !darwin

package main

import "errors"

// On other platforms the parser and renderer still work through -stdout.
// Only the clipboard bridge is macOS-specific. Linux support would shell out
// to `wl-copy --type text/html` or `xclip -selection clipboard -t text/html`.

var errNoClipboard = errors.New("clipboard access is only implemented on macOS; use -stdout")

func writeClipboard(htmlText, plainText string) error { return errNoClipboard }

func readClipboard() (string, error) { return "", errNoClipboard }
