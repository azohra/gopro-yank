package app

import (
	"bufio"
	"context"
	"encoding/json"
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
	"sync"
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
	fmt.Print(`gopro-yank — your originals, home and accounted for

Usage:
  gopro-yank <command> [options]

Commands:
  login      Capture and validate GoPro credentials
  pull       Snapshot, plan, download, hash, verify, and report
  verify     Offline SHA-256 audit; optionally reconcile source or check a replica
  list       Inspect the portable manifest
  status     Show the archive verdict
  manifest   Print or copy the canonical JSON manifest
  report     Regenerate or open the offline HTML report
  skip       Classify a source item for manual handling
  demo       Preview the transfer experience without credentials

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
	if token == "" || user == "" {
		return "", "", errors.New("missing AUTH_TOKEN and/or USER_ID; run gopro-yank login")
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
	envPath := flags.String("env-file", defaultEnvFile(), "credential file")
	noBrowser := flags.Bool("no-browser", false, "do not open gopro.com")
	force := flags.Bool("force", false, "overwrite existing credentials")
	paste := flags.Bool("paste", false, "paste values directly instead of reading the clipboard")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*envPath); err == nil && !*force {
		return errors.New("credential file exists; pass --force to overwrite it")
	}
	fmt.Println("GoPro Yank login\n\nSign in at gopro.com, then open browser DevTools → Application → Cookies → https://gopro.com.\nCopy gp_access_token and gp_user_id. Credentials stay outside the archive with owner-only permissions.")
	if !*noBrowser {
		_ = openURL(mediaLibraryURL)
	}
	reader := bufio.NewReader(os.Stdin)
	token, err := captureCredential(reader, "gp_access_token", *paste)
	if err != nil {
		return err
	}
	user, err := captureCredential(reader, "gp_user_id", *paste)
	if err != nil {
		return err
	}
	client := NewGoProClient(token, user)
	fmt.Println("Validating with GoPro...")
	profile, err := client.Validate(ctx)
	if err != nil {
		return err
	}
	if err := saveCredentials(*envPath, token, user); err != nil {
		return err
	}
	who := stringValue(profile["email"])
	if who == "" {
		who = user
	}
	fmt.Printf("Logged in as %s\nSaved to %s\nNext: gopro-yank pull\n", who, *envPath)
	return nil
}

func captureSource(ctx context.Context, client *GoProClient, archive *Archive, user string, perPage int) ([]MediaItem, error) {
	fmt.Println("Capturing the GoPro cloud inventory...")
	items, err := client.ListAll(ctx, perPage)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Preserving full source metadata for %d items...\n", len(items))
	full, err := client.FullRecords(ctx, items, 4)
	if err != nil {
		return nil, err
	}
	_, err = archive.RecordSnapshot(items, user, full)
	return items, err
}

func pullCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("pull", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "portable archive root")
	envPath := flags.String("env-file", defaultEnvFile(), "credential file")
	state := flags.String("state-dir", defaultLegacyState(), "legacy v0 marker directory")
	parallel := flags.Int("parallel", 8, "parallel item downloads")
	perPage := flags.Int("per-page", 100, "items per API page")
	ignoreSpace := flags.Bool("ignore-space-check", false, "continue if free space is below the conservative plan")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *parallel < 1 || *perPage < 1 || *perPage > 100 {
		return errors.New("--parallel must be positive and --per-page must be 1–100")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	archive, err := NewArchive(*out)
	if err != nil {
		return err
	}
	if !archive.Exists {
		if _, err := os.Stat(*state); err == nil {
			adopted, err := archive.AdoptLegacy(*state)
			if err != nil {
				return err
			}
			if adopted.AdoptedItems+adopted.AttentionItems+adopted.ManualItems > 0 {
				fmt.Printf("Adopted %d existing item(s); %d need attention (%s hashed).\n", adopted.AdoptedItems, adopted.AttentionItems, humanBytes(adopted.BytesHashed))
			}
		}
	}
	token, user, err := loadCredentials(*envPath)
	if err != nil {
		return err
	}
	client := NewGoProClient(token, user)
	if _, err := client.Validate(ctx); err != nil {
		return err
	}
	items, err := captureSource(ctx, client, archive, user, *perPage)
	if err != nil {
		return err
	}
	todo := []MediaItem{}
	manual, already := 0, 0
	var total int64
	for _, item := range items {
		if archive.NeedsDownload(item) {
			todo = append(todo, item)
			total += item.FileSize
		} else if record := archive.Item(item.ID); record != nil && record.Status == "manual" {
			manual++
		} else if archive.IsArchived(item.ID) {
			already++
		}
	}
	fmt.Printf("Cloud: %d item(s) · %d archived · %d to download (%s) · %d manual\n", len(items), already, len(todo), humanBytes(total), manual)
	if free, ok := freeDiskBytes(archive.Root); ok && !*ignoreSpace {
		overhead := int64(0)
		sizes := make([]int64, len(todo))
		for index, item := range todo {
			sizes[index] = item.FileSize
		}
		sort.Slice(sizes, func(i, j int) bool { return sizes[i] > sizes[j] })
		for index := 0; index < len(sizes) && index < *parallel; index++ {
			overhead += sizes[index]
		}
		if total+overhead > free {
			return exitError{1, fmt.Errorf("insufficient free space: %s available; conservative plan requires %s", humanBytes(free), humanBytes(total+overhead))}
		}
	}
	downloadContext, cancelDownloads := context.WithCancel(ctx)
	defer cancelDownloads()
	jobs := make(chan MediaItem)
	results := make(chan DownloadResult)
	var workers sync.WaitGroup
	started := time.Now()
	for range *parallel {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				result := downloadOne(downloadContext, client, item, archive, 5)
				if result.Status == "auth" {
					cancelDownloads()
				}
				results <- result
			}
		}()
	}
	go func() {
		for _, item := range todo {
			select {
			case jobs <- item:
			case <-downloadContext.Done():
				close(jobs)
				workers.Wait()
				close(results)
				return
			}
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()
	completed, failures := 0, 0
	var transferred int64
	for result := range results {
		completed++
		transferred += result.Bytes
		if result.Status == "fail" || result.Status == "auth" {
			failures++
			fmt.Printf("✗ %s · %s\n", result.MediaID, result.Info)
		} else {
			fmt.Printf("✓ %d/%d · %s · %s\n", completed, len(todo), result.MediaID, humanBytes(result.Bytes))
		}
	}
	if err := ctx.Err(); err != nil {
		return exitError{130, fmt.Errorf("interrupted; completed archive records remain resumable: %w", err)}
	}
	if len(todo) > 0 {
		fmt.Printf("Transferred %s in %s\n", humanBytes(transferred), time.Since(started).Round(time.Second))
	}
	verification := archive.Verify("")
	if err := archive.RecordVerification(verification); err != nil {
		return err
	}
	report, err := renderReport(archive, &verification)
	if err != nil {
		return err
	}
	printSummary(archive, &verification)
	fmt.Printf("Offline report: %s\n", report)
	if failures > 0 || !verification.OK() {
		return exitError{code: 1}
	}
	return nil
}

func requireArchive(root string) (*Archive, error) {
	archive, err := NewArchive(root)
	if err != nil {
		return nil, err
	}
	if !archive.Exists {
		return nil, fmt.Errorf("no portable archive at %s; run gopro-yank pull --out %s", archive.Root, archive.Root)
	}
	return archive, nil
}

func printSummary(archive *Archive, verification *VerificationResult) {
	summary := archive.Summary()
	fmt.Printf("Archive: %d item(s), %d file(s), %s preserved, %d manual, %d attention\n", summary.Archived, summary.Files, humanBytes(summary.Bytes), summary.Manual, summary.Blockers)
	if verification != nil {
		if verification.OK() {
			fmt.Println("Integrity: verified")
		} else {
			fmt.Printf("Integrity: %d issue(s)\n", len(verification.Issues))
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

func statusCommand(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
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
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	envPath := flags.String("env-file", defaultEnvFile(), "credential file")
	source := flags.Bool("source", false, "refresh live source first")
	replica := flags.String("replica", "", "second archive root")
	perPage := flags.Int("per-page", 100, "items per API page")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	if *source {
		token, user, err := loadCredentials(*envPath)
		if err != nil {
			return err
		}
		client := NewGoProClient(token, user)
		if _, err := client.Validate(ctx); err != nil {
			return err
		}
		if _, err := captureSource(ctx, client, archive, user, *perPage); err != nil {
			return err
		}
	}
	root := *replica
	result := archive.Verify(root)
	if root == "" {
		if err := archive.RecordVerification(result); err != nil {
			return err
		}
		if _, err := renderReport(archive, &result); err != nil {
			return err
		}
	}
	fmt.Printf("Checked %d item(s), %d file(s), %s\n", result.CheckedItems, result.CheckedFiles, humanBytes(result.CheckedBytes))
	for _, issue := range result.Issues {
		fmt.Printf("%s · %s · %s · %s\n", issue.MediaID, issue.Kind, issue.Path, issue.Message)
	}
	printSummary(archive, &result)
	if !result.OK() || archive.Summary().Blockers > 0 {
		return exitError{code: 1}
	}
	return nil
}

func listCommand(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	pending := flags.Bool("pending", false, "only items needing attention")
	done := flags.Bool("done", false, "only verified archived items")
	asJSON := flags.Bool("json", false, "JSON lines")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *pending && *done {
		return errors.New("--pending and --done are mutually exclusive")
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(archive.Data.Items))
	for id := range archive.Data.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		record := archive.Data.Items[id]
		bad := record.Integrity != nil && record.Integrity.Status == "failed"
		if *pending && record.Status == "archived" && !bad {
			continue
		}
		if *done && (record.Status != "archived" || bad) {
			continue
		}
		if *asJSON {
			payload, _ := json.Marshal(record)
			fmt.Println(string(payload))
		} else {
			fmt.Printf("%-10s %-10s %-24s %10s  %s\n", record.Status, firstDate(record.CapturedAt, record.CreatedAt), record.Filename, humanBytes(record.FileSize), record.MediaID)
		}
	}
	return nil
}
func firstDate(values ...string) string {
	for _, value := range values {
		if len(value) >= 10 {
			return value[:10]
		}
	}
	return "—"
}

func manifestCommand(args []string) error {
	flags := flag.NewFlagSet("manifest", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	outFile := flags.String("out-file", "", "write a copy")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(archive.Data, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if *outFile != "" {
		return atomicWrite(*outFile, payload, 0o644)
	}
	fmt.Print(string(payload))
	return nil
}

func reportCommand(args []string) error {
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	open := flags.Bool("open", false, "open report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	path, err := renderReport(archive, nil)
	if err != nil {
		return err
	}
	fmt.Println(path)
	if *open {
		return openURL("file://" + filepath.ToSlash(path))
	}
	return nil
}

func skipCommand(args []string) error {
	flags := flag.NewFlagSet("skip", flag.ContinueOnError)
	out := flags.String("out", defaultArchiveRoot(), "archive root")
	reason := flags.String("reason", "", "classification reason")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *reason == "" || flags.NArg() == 0 {
		return errors.New("--reason and at least one media ID are required")
	}
	archive, err := requireArchive(*out)
	if err != nil {
		return err
	}
	for _, id := range flags.Args() {
		if err := archive.MarkManual(id, *reason); err != nil {
			return err
		}
		fmt.Printf("manual %s · %s\n", id, *reason)
	}
	_, err = renderReport(archive, nil)
	return err
}

func demoCommand(args []string) error {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	count := flags.Int("count", 12, "simulated items")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *count < 1 {
		return errors.New("count must be positive")
	}
	total := int64(0)
	for index := 1; index <= *count; index++ {
		total += int64(128+index*17) * 1024 * 1024
	}
	fmt.Printf("GOPRO YANK / DEMO\nYour originals. Home and accounted for.\n\nsource    %d simulated originals · %s\narchive   portable + resumable\n\n", *count, humanBytes(total))
	for index := 1; index <= *count; index++ {
		size := int64(128+index*17) * 1024 * 1024
		fmt.Printf("✓ %02d/%02d  demo-%03d  %9s  SHA-256\n", index, *count, index, humanBytes(size))
		time.Sleep(90 * time.Millisecond)
	}
	fmt.Println("\nproof     manifest · checksums · offline report")
	fmt.Println("verdict   DOWNLOADABLE MEDIA EXPORT COMPLETE")
	return nil
}

func run(ctx context.Context, args []string, version string) error {
	if len(args) == 0 {
		archive, err := NewArchive(defaultArchiveRoot())
		if err != nil {
			return err
		}
		if archive.Exists {
			printSummary(archive, nil)
			return nil
		}
		if _, _, err := loadCredentials(defaultEnvFile()); err == nil {
			fmt.Println("GoPro Yank is configured.\nNo default archive exists yet.\nNext: gopro-yank pull")
		} else {
			fmt.Println("GoPro Yank is ready.\nNo credentials or default archive exist yet.\nNext: gopro-yank login")
		}
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return nil
	case "-v", "--version", "version":
		fmt.Printf("gopro-yank %s\n", version)
		return nil
	case "login":
		return loginCommand(ctx, args[1:])
	case "pull":
		return pullCommand(ctx, args[1:])
	case "verify":
		return verifyCommand(ctx, args[1:])
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
