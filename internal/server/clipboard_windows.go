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

// clipboardImageScript is a constant — the file path is passed in as a
// PowerShell parameter via the argument vector, NOT interpolated into the
// script string. This eliminates any possibility of command injection from a
// crafted filename containing backticks, $(...), newlines, quotes, etc.
const clipboardImageScript = `
param([Parameter(Mandatory=$true)][string]$Path)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$data = New-Object System.Windows.Forms.DataObject

# 1. File drop — original bytes, zero re-encode. Target: Discord, Slack, Word, Chrome upload, etc.
$files = New-Object System.Collections.Specialized.StringCollection
[void]$files.Add($Path)
$data.SetFileDropList($files)

# 2. Bitmap — for Paint/Photoshop which only accept raw pixels. Loaded from a
#    memory copy so the file isn't locked after we return.
try {
  $bytes = [System.IO.File]::ReadAllBytes($Path)
  $ms = New-Object System.IO.MemoryStream(,$bytes)
  $img = [System.Drawing.Image]::FromStream($ms)
  $data.SetImage($img)
} catch {
  # non-decodable (e.g. HEIC without codec) — file drop still works
}

[System.Windows.Forms.Clipboard]::SetDataObject($data, $true)
`

// copyImageToClipboard puts an image on the Windows clipboard losslessly.
// See clipboardImageScript for details on the dual representation.
func copyImageToClipboard(path string) {
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-STA",
		"-ExecutionPolicy", "Bypass",
		"-Command", clipboardImageScript,
		"-Path", path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("clipboard image: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}
