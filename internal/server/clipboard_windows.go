//go:build windows

package server

import (
	"log"
	"os/exec"
	"strings"
)

func copyToClipboard(text string) {
	cmd := exec.Command("clip.exe")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		log.Printf("clipboard text: %v", err)
	}
}

// clipboardImageScript reads the target path from the CLIPMUXD_IMG environment
// variable — NOT from a script parameter and NOT interpolated into the script
// text. Passing via env var is the most robust way to hand an arbitrary string
// into a `powershell -Command` invocation: there's no arg-parsing involved and
// no shell quoting to get wrong, so a malicious filename can't break out.
const clipboardImageScript = `
$ErrorActionPreference = 'Stop'
$path = $env:CLIPMUXD_IMG
if (-not $path) { exit 2 }

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$data = New-Object System.Windows.Forms.DataObject

# 1. File drop — original bytes, zero re-encode. Target: Discord, Slack, Word, Chrome upload, etc.
$files = New-Object System.Collections.Specialized.StringCollection
[void]$files.Add($path)
$data.SetFileDropList($files)

# 2. Bitmap — for Paint/Photoshop which only accept raw pixels. Loaded from a
#    memory copy so the file isn't locked after we return.
try {
  $bytes = [System.IO.File]::ReadAllBytes($path)
  $ms = New-Object System.IO.MemoryStream(,$bytes)
  $img = [System.Drawing.Image]::FromStream($ms)
  $data.SetImage($img)
} catch {
  # non-decodable (e.g. HEIC without codec) — file drop still works
}

[System.Windows.Forms.Clipboard]::SetDataObject($data, $true)
`

// copyImageToClipboard puts an image on the Windows clipboard losslessly.
// Dual-representation: file drop (target apps get original bytes) + bitmap
// (Paint/Photoshop get raw pixels).
func copyImageToClipboard(path string) {
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-STA",
		"-ExecutionPolicy", "Bypass",
		"-Command", clipboardImageScript,
	)
	cmd.Env = append(cmd.Environ(), "CLIPMUXD_IMG="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("clipboard image: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}
