//go:build darwin

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Clipboard access goes through osascript running JavaScript for Automation
// (JXA), which can call AppKit's NSPasteboard directly. Alternatives
// considered:
//
//   - pbcopy only writes plain text, so pasted HTML shows up as literal
//     tags. pbpaste decodes using the caller's locale and mangles non-ASCII
//     text when LANG is unset.
//   - cgo plus Objective-C would need Xcode command-line tools to build and
//     gives up trivial cross-compilation, all for two API calls.
//
// osascript ships with every macOS install, so the binary stays pure Go.
//
// Payloads travel over stdin as JSON, never as argv. That avoids ARG_MAX
// limits on large tables and any quoting or escaping of user data into
// script source. Both scripts use a run() handler: osascript prints the
// value of a top-level script's last expression, and a run() that returns
// nothing keeps stdout clean.

// writeScript sets two representations in one pasteboard write:
// public.html for rich-text targets, and public.utf8-plain-text as the
// fallback for everything else. clearContents comes first because
// NSPasteboard only accepts new types after the change count is bumped.
const writeScript = `
ObjC.import("AppKit");
function run() {
  const data = $.NSFileHandle.fileHandleWithStandardInput.readDataToEndOfFile;
  const p = JSON.parse(ObjC.unwrap($.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding)));
  const pb = $.NSPasteboard.generalPasteboard;
  pb.clearContents;
  if (!pb.setStringForType($(p.html), "public.html")) throw new Error("pasteboard rejected public.html");
  pb.setStringForType($(p.text), "public.utf8-plain-text");
}`

// readScript writes the clipboard's plain text to stdout as raw UTF-8
// bytes. It uses NSFileHandle rather than returning the string, which would
// let osascript re-encode it and append a newline.
const readScript = `
ObjC.import("AppKit");
function run() {
  const s = $.NSPasteboard.generalPasteboard.stringForType("public.utf8-plain-text");
  if (s.isNil()) return;
  $.NSFileHandle.fileHandleWithStandardOutput.writeData(s.dataUsingEncoding($.NSUTF8StringEncoding));
}`

func writeClipboard(htmlText, plainText string) error {
	payload, err := json.Marshal(map[string]string{"html": htmlText, "text": plainText})
	if err != nil {
		return err
	}
	_, err = osascript(writeScript, payload)
	return err
}

func readClipboard() (string, error) {
	out, err := osascript(readScript, nil)
	return string(out), err
}

// osascript runs a JXA script with stdin as input. It returns stdout, and
// on failure includes stderr in the error, since that is where JXA reports
// exceptions.
func osascript(script string, stdin []byte) ([]byte, error) {
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", script)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("osascript: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
