package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"

	"clipmuxd/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	inbox := flag.String("inbox", defaultInbox(), "inbox directory")
	tokenFlag := flag.String("token", "", "auth token (generated on first run if empty)")
	showURL := flag.Bool("url", false, "print phone URL and exit")
	flag.Parse()

	if err := os.MkdirAll(*inbox, 0o755); err != nil {
		log.Fatalf("inbox: %v", err)
	}

	tok := *tokenFlag
	if tok == "" {
		tok = loadOrCreateToken(*inbox)
	}

	if *showURL {
		printURLs(*addr, tok)
		return
	}

	log.Printf("clipmuxd starting")
	log.Printf("inbox: %s", *inbox)
	printURLs(*addr, tok)

	srv := server.New(server.Config{
		Addr:   *addr,
		Inbox:  *inbox,
		Token:  tok,
	})
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

func defaultInbox() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "inbox"
	}
	return filepath.Join(home, "clipmuxd", "inbox")
}

func loadOrCreateToken(inbox string) string {
	tokPath := filepath.Join(filepath.Dir(inbox), "token")
	if b, err := os.ReadFile(tokPath); err == nil && len(b) >= 16 {
		return string(b)
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		log.Fatal(err)
	}
	tok := hex.EncodeToString(buf)
	_ = os.MkdirAll(filepath.Dir(tokPath), 0o755)
	if err := os.WriteFile(tokPath, []byte(tok), 0o600); err != nil {
		log.Fatalf("write token: %v", err)
	}
	return tok
}

func printURLs(addr, tok string) {
	_, port, _ := net.SplitHostPort(addr)
	if port == "" {
		port = addr
	}
	fmt.Println("------------------------------------------------------------")
	fmt.Println("Open on your phone (same Wi-Fi):")
	for _, ip := range localIPs() {
		fmt.Printf("  http://%s:%s/?k=%s\n", ip, port, tok)
	}
	fmt.Println()
	fmt.Println("For remote access, install Tailscale on PC + phone, then:")
	fmt.Printf("  http://<your-tailscale-ip>:%s/?k=%s\n", port, tok)
	fmt.Println("------------------------------------------------------------")
}

func localIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			out = append(out, ipnet.IP.String())
		}
	}
	return out
}
