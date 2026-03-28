# hme — iCloud Hide My Email CLI

Generate, reserve, and list iCloud Hide My Email aliases from the command line.

## Install

### Homebrew

```bash
brew install mark-liu/tap/hme
```

### From release

Download a pre-built binary from [Releases](https://github.com/mark-liu/hme/releases):

```bash
# macOS (Apple Silicon)
curl -Lo hme https://github.com/mark-liu/hme/releases/latest/download/hme_darwin_arm64
chmod +x hme && mv hme /usr/local/bin/
```

### From source

```bash
go install github.com/mark-liu/hme@latest
```

## Setup

hme authenticates using iCloud web session cookies extracted from Chrome.

### Prerequisites

1. Sign in to [icloud.com](https://www.icloud.com) in Chrome (one time)
2. Run `hme auth`

That's it. hme reads cookies directly from Chrome's cookie store.

### How auth works

```bash
hme auth
# Reading Chrome Safe Storage key...
# [macOS Keychain prompt appears]
# Extracted 10 cookies from Chrome.
# Cookies saved and validated.
```

On first run, macOS will show a Keychain password dialog asking to access "Chrome Safe Storage". Click **"Always Allow"** to grant permanent access — subsequent runs will be completely silent with no prompt.

If you click "Allow" instead of "Always Allow", you'll get the password prompt every time.

hme auto-detects your Chrome profile, including custom `--user-data-dir` profiles (e.g. for Playwright/CDP setups). To target a specific profile:

```bash
hme auth --profile "Profile 3"
```

### Manual cookie paste (fallback)

If you don't use Chrome, you can paste cookies manually from any browser's DevTools:

```bash
hme auth --manual
```

### Cookie lifetime

Cookies typically last **2-4 weeks** if you checked "Keep me signed in" on icloud.com. When they expire, you'll see:

```
Error: cookies expired. Run 'hme auth' to refresh.
```

Just run `hme auth` again.

Cookies are stored at `~/.config/hme/cookies.txt` with `0600` permissions.

## Usage

### Generate a new alias

```bash
hme generate "GitHub"
# random123@icloud.com (copied to clipboard)
# Label: GitHub

hme gen "Shopping" "throwaway for deals"
# random456@icloud.com (copied to clipboard)
# Label: Shopping
# Note:  throwaway for deals
```

The generated email is automatically copied to your clipboard.

### List aliases

```bash
# Active aliases (table format)
hme list

# Filter by regex
hme list --search "git"

# Include inactive aliases
hme list --inactive

# JSON output
hme list --json

# Combine filters
hme list --search "shop" --inactive --json
```

### Aliases

| Command | Short forms |
|---------|-------------|
| `generate` | `gen`, `g` |
| `list` | `ls`, `l` |

## How it works

hme talks to Apple's private iCloud API (`p68-maildomainws.icloud.com`), the same API that Safari and iCloud.com use. It:

1. **Generates** a random `@icloud.com` address (`POST /v1/hme/generate`)
2. **Reserves** it with your label (`POST /v1/hme/reserve`)
3. **Lists** all your aliases (`GET /v2/hme/list`)

No Apple-provided public API exists for this. The tool reverse-engineers the same endpoints the web UI uses.

## Security

**Your iCloud cookies grant full access to your Apple account. Treat them like a password.**

- Cookies are stored at `~/.config/hme/cookies.txt` with `0600` file permissions
- The config directory is created with `0700` permissions
- Cookie values and dsid are never logged or printed in error messages
- No telemetry, analytics, or network calls except to Apple's API
- No secrets in source code

If your cookies are compromised, sign out of iCloud.com to invalidate all web sessions.

## Project layout

```
main.go          Entry point, subcommand dispatch
client.go        iCloud API client (HTTP, headers, 3 endpoints)
client_test.go   httptest-based unit tests
types.go         Request/response structs with JSON tags
config.go        Cookie storage, dsid extraction, stdin prompts
config_test.go   Config layer tests
browser.go       Chrome cookie extraction (Keychain + AES-CBC decryption)
browser_test.go  Decryption round-trip tests
clipboard.go     Cross-platform clipboard (pbcopy/xclip/wl-copy/clip.exe)
table.go         Tabwriter-based table formatter for list output
```

Single `main` package, flat layout. Two external dependencies (`golang.org/x/term`, `golang.org/x/crypto`).

## Development

```bash
go test ./...
go vet ./...
go build -o hme .
```

## License

MIT
