package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func defaultArchiveRoot() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return filepath.Join(home, "Pictures", "GoPro")
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "GoPro-Archive")
}
func defaultEnvFile() string {
	config, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "gopro-yank", ".env")
}
func defaultLegacyState() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "gopro-yank", "state")
}

const mediaLibraryURL = "https://gopro.com/media-library/"

func usage() {
	fmt.Print(`gopro-yank — bring your GoPro library home

Usage:
  gopro-yank                    Open the interactive app
  gopro-yank library [options]  Inspect your library without downloading
  gopro-yank archive [options]  Archive or resume available originals
  gopro-yank verify [options]   Check every archived file

Account:
  gopro-yank login [options]    Connect without the interactive app

Run gopro-yank <command> --help for command options.
`)
}

func loadCredentials(path string) (string, string, error) {
	values := map[string]string{}
	if payload, err := os.ReadFile(path); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(payload)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			values[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		}
	}
	token := os.Getenv("AUTH_TOKEN")
	if token == "" {
		token = values["AUTH_TOKEN"]
	}
	user := os.Getenv("USER_ID")
	if user == "" {
		user = values["USER_ID"]
	}
	if token == "" {
		return "", "", errors.New("GoPro is not connected; run gopro-yank login")
	}
	return token, user, nil
}

func saveCredentials(path, token, user string) error {
	return atomicWrite(path, []byte("AUTH_TOKEN="+token+"\nUSER_ID="+user+"\n"), 0o600)
}

func openURL(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	return command.Start()
}

func readPrompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	return strings.TrimSpace(value), err
}

func readClipboard() string {
	var commands [][]string
	switch runtime.GOOS {
	case "darwin":
		commands = [][]string{{"pbpaste"}}
	case "windows":
		commands = [][]string{{"powershell", "-NoProfile", "-Command", "Get-Clipboard"}}
	default:
		commands = [][]string{{"wl-paste", "--no-newline"}, {"xclip", "-selection", "clipboard", "-o"}, {"xsel", "--clipboard", "--output"}}
	}
	for _, parts := range commands {
		if _, err := exec.LookPath(parts[0]); err != nil {
			continue
		}
		if output, err := exec.Command(parts[0], parts[1:]...).Output(); err == nil {
			return strings.TrimSpace(string(output))
		}
	}
	return ""
}

func captureCredential(reader *bufio.Reader, label string, paste bool) (string, error) {
	if !paste {
		fmt.Printf("Copy %s in the browser, then press Enter here: ", label)
		if _, err := reader.ReadString('\n'); err != nil {
			return "", err
		}
		if value := readClipboard(); value != "" {
			fmt.Printf("captured %d characters\n", len(value))
			return value, nil
		}
		fmt.Println("Clipboard unavailable; paste directly instead.")
	}
	return readPrompt(reader, label+": ")
}

func loginCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	envPath := flags.String("env-file", defaultEnvFile(), "saved GoPro login")
	noBrowser := flags.Bool("no-browser", false, "paste a login token instead")
	paste := flags.Bool("paste", false, "paste the login token instead of reading the clipboard")
	if err := flags.Parse(args); err != nil {
		return err
	}
	fmt.Println("GoPro Yank login")
	var token string
	var err error
	if !*noBrowser && !*paste {
		fmt.Println("\nOpening a separate GoPro sign-in window...\nSign in there. GoPro Yank never sees your password.")
		token, err = loginInBrowser(ctx)
		if err != nil {
			return fmt.Errorf("automatic sign-in failed: %w; use gopro-yank login --no-browser for the manual option", err)
		}
	} else {
		fmt.Println("\nSign in at gopro.com, then copy gp_access_token from the browser's cookies.\nYou may also paste a Cookie header containing it.")
		if !*noBrowser {
			_ = openURL(mediaLibraryURL)
		}
		reader := bufio.NewReader(os.Stdin)
		value, captureErr := captureCredential(reader, "gp_access_token", *paste)
		if captureErr != nil {
			return captureErr
		}
		token = parseCredential(value, "gp_access_token")
		if token == "" {
			return errors.New("gp_access_token was not found")
		}
	}
	fmt.Println("Checking the connection...")
	_, user, err := connectGoPro(ctx, token, "")
	if err != nil {
		return err
	}
	if err := saveCredentials(*envPath, token, user); err != nil {
		return err
	}
	fmt.Println("Connected to GoPro\nLogin saved on this computer\nNext: gopro-yank library")
	return nil
}

func connectGoPro(ctx context.Context, token, user string) (*GoProClient, string, error) {
	client := NewGoProClient(token)
	profile, err := client.Validate(ctx)
	if err != nil {
		return nil, "", err
	}
	if user == "" {
		user = profileUserID(profile)
	}
	if user == "" {
		return nil, "", errors.New("GoPro accepted the login but did not identify the account")
	}
	return client, user, nil
}

func archiveCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("archive", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive folder")
	envPath := flags.String("env-file", defaultEnvFile(), "saved GoPro login")
	state := flags.String("state-dir", defaultLegacyState(), "older Python v0 records folder")
	parallel := flags.Int("parallel", 8, "downloads to run at once")
	perPage := flags.Int("per-page", 100, "GoPro items requested at once")
	ignoreSpace := flags.Bool("ignore-space-check", false, "continue when the disk-space check fails")
	if err := flags.Parse(args); err != nil {
		return err
	}
	lastStage := ""
	result, err := ArchiveLibrary(ctx, ArchiveOptions{
		Root:             *out,
		EnvPath:          *envPath,
		LegacyState:      *state,
		Parallel:         *parallel,
		PerPage:          *perPage,
		IgnoreSpaceCheck: *ignoreSpace,
	}, func(event ArchiveEvent) {
		if event.Stage != "" && event.Stage != lastStage {
			fmt.Println(event.Stage + "...")
			lastStage = event.Stage
		}
		if event.Result != nil {
			if event.Result.Status == "fail" || event.Result.Status == "auth" {
				fmt.Printf("✗ %s · %s\n", event.Result.MediaID, event.Result.Info)
			} else {
				fmt.Printf("✓ %d/%d · %s · %s\n", event.Current, event.Total, event.Result.MediaID, humanBytes(event.Result.Bytes))
			}
		}
		if event.Inspection != nil {
			inspection := event.Inspection
			fmt.Printf("GoPro: %d original(s) · %d archived · %d to archive (%s) · %d need manual export\n", inspection.Total, inspection.Archived, inspection.Remaining, humanBytes(inspection.RemainingBytes), inspection.Manual)
		}
	})
	if result.Transferred > 0 {
		fmt.Printf("Transferred %s in %s\n", humanBytes(result.Transferred), result.Elapsed.Round(time.Second))
	}
	if result.ReportPath != "" {
		printSummaryValues(result.Summary, &result.Verification)
		fmt.Printf("Offline report: %s\n", result.ReportPath)
	}
	if errors.Is(err, ErrArchiveIncomplete) {
		return exitError{code: 1}
	}
	if err != nil && ctx.Err() != nil {
		return exitError{130, err}
	}
	return err
}

func pullCommand(ctx context.Context, args []string) error { return archiveCommand(ctx, args) }

func printLibraryInspection(inspection LibraryInspection) {
	fmt.Printf("GoPro library: %d original(s), %s", inspection.Total, humanBytes(inspection.TotalBytes))
	if inspection.Earliest != "" {
		fmt.Printf(" · %s to %s", inspection.Earliest, inspection.Latest)
	}
	fmt.Println()
	kinds := make([]string, 0, len(inspection.Types))
	for kind := range inspection.Types {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		fmt.Printf("  %-18s %d\n", kind, inspection.Types[kind])
	}
	fmt.Printf("Archive: %d saved · %d remaining (%s) · %d need manual export\n", inspection.Archived, inspection.Remaining, humanBytes(inspection.RemainingBytes), inspection.Manual)
	fmt.Println("Nothing was downloaded.")
}

func libraryCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("library", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive folder")
	envPath := flags.String("env-file", defaultEnvFile(), "saved GoPro login")
	perPage := flags.Int("per-page", 100, "GoPro items requested at once")
	if err := flags.Parse(args); err != nil {
		return err
	}
	fmt.Println("Reading your GoPro library...")
	inspection, err := InspectLibrary(ctx, *out, *envPath, *perPage)
	if err != nil {
		return err
	}
	printLibraryInspection(inspection)
	fmt.Println("Next: gopro-yank archive")
	return nil
}

func requireArchive(root string) (*Archive, error) {
	archive, err := NewArchive(root)
	if err != nil {
		return nil, err
	}
	if !archive.Exists {
		return nil, fmt.Errorf("no portable archive at %s; run gopro-yank archive --out %s", archive.Root, archive.Root)
	}
	return archive, nil
}

func printSummaryValues(summary Summary, verification *VerificationResult) {
	fmt.Printf("Archive: %d original(s), %d file(s), %s saved, %d need manual export, %d need attention\n", summary.Archived, summary.Files, humanBytes(summary.Bytes), summary.Manual, summary.Blockers)
	if verification != nil {
		if verification.OK() {
			fmt.Println("File check: passed")
		} else {
			fmt.Printf("File check: %d issue(s)\n", len(verification.Issues))
		}
	}
	if summary.Archived == 0 {
		fmt.Println("NO MEDIA ARCHIVED")
	} else if summary.Blockers > 0 || (verification != nil && !verification.OK()) {
		fmt.Println("MEDIA EXPORT NEEDS ATTENTION")
	} else {
		fmt.Println("DOWNLOADABLE MEDIA EXPORT COMPLETE")
	}
}

func printSummary(archive *Archive, verification *VerificationResult) {
	printSummaryValues(archive.Summary(), verification)
}

func statusCommand(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive folder")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	printSummary(archive, nil)
	return nil
}

func verifyCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive folder")
	envPath := flags.String("env-file", defaultEnvFile(), "saved GoPro login")
	source := flags.Bool("source", false, "compare with your current GoPro library")
	replica := flags.String("replica", "", "copied archive folder")
	perPage := flags.Int("per-page", 100, "GoPro items requested at once")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	if *source {
		client, user, err := connectFromFile(ctx, *envPath)
		if err != nil {
			return err
		}
		if _, err := saveSourceSnapshot(ctx, client, archive, user, *perPage, func(event ArchiveEvent) {
			if event.Stage != "" {
				fmt.Println(event.Stage + "...")
			}
		}); err != nil {
			return err
		}
	}
	result, verifyErr := VerifyArchive(ctx, *out, *replica)
	fmt.Printf("Checked %d item(s), %d file(s), %s\n", result.Verification.CheckedItems, result.Verification.CheckedFiles, humanBytes(result.Verification.CheckedBytes))
	for _, issue := range result.Verification.Issues {
		fmt.Printf("%s · %s · %s · %s\n", issue.MediaID, issue.Kind, issue.Path, issue.Message)
	}
	printSummaryValues(result.Summary, &result.Verification)
	if errors.Is(verifyErr, ErrArchiveIncomplete) {
		return exitError{code: 1}
	}
	return verifyErr
}

func run(ctx context.Context, args []string, version string) error {
	if len(args) == 0 {
		if terminalIsInteractive() {
			return runTUI(ctx, version, false)
		}
		usage()
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return nil
	case "-v", "--version", "version":
		fmt.Printf("gopro-yank %s\n", version)
		return nil
	case "--demo":
		if terminalIsInteractive() {
			return runTUI(ctx, version, true)
		}
		return demoCommand(args[1:])
	case "library":
		return libraryCommand(ctx, args[1:])
	case "archive":
		return archiveCommand(ctx, args[1:])
	case "verify":
		return verifyCommand(ctx, args[1:])
	case "login":
		return loginCommand(ctx, args[1:])
	// Compatibility aliases and advanced archive tools remain scriptable.
	case "pull":
		return pullCommand(ctx, args[1:])
	case "list":
		return listCommand(args[1:])
	case "status":
		return statusCommand(args[1:])
	case "manifest":
		return manifestCommand(args[1:])
	case "report":
		return reportCommand(args[1:])
	case "skip":
		return skipCommand(args[1:])
	case "demo":
		if terminalIsInteractive() {
			return runTUI(ctx, version, true)
		}
		return demoCommand(args[1:])
	default:
		usage()
		return exitError{2, fmt.Errorf("unknown command: %s", strconv.Quote(args[0]))}
	}
}

// Main runs the CLI and returns a process exit code.
func Main(ctx context.Context, args []string, version string) int {
	err := run(ctx, args, version)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	var exit exitError
	if errors.As(err, &exit) {
		if exit.err != nil {
			fmt.Fprintln(os.Stderr, "gopro-yank:", exit.err)
		}
		return exit.code
	}
	fmt.Fprintln(os.Stderr, "gopro-yank:", err)
	return 1
}
