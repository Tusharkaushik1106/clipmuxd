//go:build !windows

package server

func copyToClipboard(text string)      {}
func copyImageToClipboard(path string) {}
