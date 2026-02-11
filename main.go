package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch strings.ToLower(os.Args[1]) {
	case "generate", "gen", "g":
		runGenerate(os.Args[2:])
	case "list", "ls", "l":
		runList(os.Args[2:])
	case "auth":
		runAuth(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("hme %s (%s)\n", version, commit)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `hme — iCloud Hide My Email CLI

Usage:
  hme generate <label> [note]    Generate and reserve a new HME alias
  hme list [flags]               List HME aliases
  hme auth [flags]               Store iCloud cookies (auto-extracts from Chrome)
  hme version                    Print version

Generate aliases:
  hme generate "GitHub"
  hme gen "Shopping" "throwaway for deals"

List aliases:
  hme list                       Active aliases (table)
  hme list --search "git"        Filter by regex
  hme list --inactive            Include inactive aliases
  hme list --json                Output as JSON

Auth:
  hme auth                       Extract cookies from Chrome (Touch ID)
  hme auth --manual              Paste cookies manually
  hme auth --profile "Profile 3" Use a specific Chrome profile

Aliases:
  generate: gen, g
  list:     ls, l
`)
}

func runGenerate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: hme generate <label> [note]")
		os.Exit(2)
	}
	label := args[0]
	note := ""
	if len(args) > 1 {
		note = args[1]
	}

	client, err := clientFromConfig()
	if err != nil {
		fatal(err)
	}

	email, err := client.Generate()
	if err != nil {
		fatal(err)
	}

	if err := client.Reserve(email, label, note); err != nil {
		fatal(err)
	}

	copied := ""
	if err := CopyToClipboard(email); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	} else if HasClipboard() {
		copied = " (copied to clipboard)"
	}

	fmt.Printf("%s%s\n", email, copied)
	fmt.Printf("Label: %s\n", label)
	if note != "" {
		fmt.Printf("Note:  %s\n", note)
	}
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	search := fs.String("search", "", "filter by regex (matches email, label, note)")
	inactive := fs.Bool("inactive", false, "include inactive aliases")
	jsonOut := fs.Bool("json", false, "output as JSON")
	fs.Parse(args)

	client, err := clientFromConfig()
	if err != nil {
		fatal(err)
	}

	emails, err := client.List()
	if err != nil {
		fatal(err)
	}

	// Filter active-only by default
	if !*inactive {
		filtered := emails[:0]
		for _, e := range emails {
			if e.IsActive {
				filtered = append(filtered, e)
			}
		}
		emails = filtered
	}

	// Filter by search regex
	if *search != "" {
		re, err := regexp.Compile("(?i)" + *search)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid regex %q: %v\n", *search, err)
			os.Exit(2)
		}
		filtered := emails[:0]
		for _, e := range emails {
			if re.MatchString(e.Hme) || re.MatchString(e.Label) || re.MatchString(e.Note) {
				filtered = append(filtered, e)
			}
		}
		emails = filtered
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(emails)
		return
	}

	if len(emails) == 0 {
		fmt.Println("No aliases found.")
		return
	}

	WriteTable(os.Stdout, emails)
	fmt.Fprintf(os.Stderr, "\n%d alias(es)\n", len(emails))
}

func runAuth(args []string) {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	manual := fs.Bool("manual", false, "paste cookies manually instead of extracting from Chrome")
	profile := fs.String("profile", "", "Chrome profile directory name (e.g. \"Profile 3\")")
	fs.Parse(args)

	var cookies string
	var err error

	if *manual {
		cookies, err = runAuthManual()
	} else {
		cookies, err = ExtractChromeCookies(*profile)
	}
	if err != nil {
		fatal(err)
	}

	if _, err := ExtractDSID(cookies); err != nil {
		fatal(fmt.Errorf("invalid cookies: %w", err))
	}

	if err := SaveCookies(cookies); err != nil {
		fatal(err)
	}

	p, _ := cookiePath()
	fmt.Printf("Cookies saved and validated.\nStored at: %s\n", p)
}

func runAuthManual() (string, error) {
	fmt.Fprint(os.Stderr, `To get your iCloud cookie string:

  Safari:
  1. Enable Develop menu: Safari → Settings → Advanced → "Show features for web developers"
  2. Open https://www.icloud.com and sign in
  3. Open Web Inspector: Develop → Show Web Inspector (or Cmd+Option+I)
  4. Go to the Network tab
  5. Click "Hide My Email" in iCloud settings (to trigger an API request)
  6. Filter by "maildomainws" to find a request to p68-maildomainws.icloud.com
  7. Click the request → Headers → scroll to "Request Headers"
  8. Copy the full "Cookie" header value (double-click to select, then Cmd+C)

  Chrome / Firefox:
  1. Open https://www.icloud.com and sign in
  2. Open DevTools: Cmd+Option+I (Mac) or F12 (Windows/Linux)
  3. Go to the Network tab
  4. Click "Hide My Email" in iCloud settings (to trigger an API request)
  5. Filter by "maildomainws" to find a request to p68-maildomainws.icloud.com
  6. Click the request → Headers → scroll to "Request Headers"
  7. Right-click the "Cookie" header value → Copy value

The cookie string looks like:
  X-APPLE-WEBAUTH-USER=d12345...; X-APPLE-WEBAUTH-TOKEN=...; ...

`)
	return PromptForCookies()
}

// clientFromConfig loads cookies and returns a ready Client.
func clientFromConfig() (*Client, error) {
	cookies, err := LoadCookies()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no cookies configured. Run 'hme auth' first")
		}
		return nil, err
	}
	return NewClient(cookies)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
