//go:build !darwin

package main

import "errors"

// On other platforms the parser and renderer still work through -stdout.
// Only the clipboard bridge is macOS-specific. Linux support would shell out
// to `wl-copy --type text/html` or `xclip -selection clipboard -t text/html`.

// The read and write paths need different advice, so they get separate
// errors. A failed write is fixed by -stdout. A failed read happens before
// -stdout matters at all (no file and nothing piped in), so suggesting
// -stdout there would point at a flag the user may already have passed.
// main.go prefixes read errors with "reading clipboard: ", which is why
// errNoClipboardRead does not say "reading" itself.
var (
	errNoClipboard     = errors.New("clipboard access is only implemented on macOS; use -stdout")
	errNoClipboardRead = errors.New("clipboard access is only implemented on macOS; pass a file or pipe the text in")
)

func writeClipboard(htmlText, plainText string) error { return errNoClipboard }

func readClipboard() (string, error) { return "", errNoClipboardRead }
