# Security

## Threat model

clipmuxd is a personal tool meant to run on a trusted PC and be accessed
only by the owner's own phone (over LAN or a private Tailscale tunnel).

**In scope:**
- Protecting the inbox and server from unauthorized access by other devices on the LAN.
- Protecting the filesystem from path traversal / command injection via crafted uploads.
- Protecting against XSS / CSRF from the UI.

**Out of scope:**
- Physical access to the PC (attacker with local login already wins).
- Public-internet exposure (don't port-forward this; use Tailscale).
- A compromised phone that already holds the token.

## What protects you

- **24-byte cryptographically random token** from `crypto/rand`, required on every API request.
- **Path traversal blocked:** every ID parameter runs through `filepath.Base` and is rejected if it still contains `..`.
- **Command injection blocked:** the PowerShell clipboard helper passes the file path as a parameter argument (not interpolated into the script). Filenames cannot be executed as code.
- **XSS blocked:** all server-supplied strings pass through an HTML escaper before `innerHTML`. Media thumbnails use `encodeURIComponent` on IDs.
- **CSRF mitigated:** auth cookie is `HttpOnly` + `SameSite=Lax`. Mutating endpoints require `POST`, which Lax blocks from third-party contexts.
- **Upload cap:** 5 GB per request via `http.MaxBytesReader`.
- **File collisions:** `O_CREATE|O_EXCL` with counter fallback — concurrent uploads can't silently overwrite.
- **No telemetry, no external calls.** The server speaks only to your phone.

## Known limitations

- **Plain HTTP on LAN.** Fine on home Wi-Fi; use Tailscale (WireGuard) for anything outside your trusted network.
- **Token is visible in the URL bar.** Don't screenshot it. If it leaks, delete the `token` file in the clipmuxd data directory and restart the server to rotate.
- **No per-sender disk quota.** A malicious sender with the token could fill your disk.
- **Single-user model.** There is no ACL system; whoever has the token has full access.

## Reporting a vulnerability

Open a private security advisory on GitHub, or email the maintainer. Please
don't file public issues for unpatched vulnerabilities.
