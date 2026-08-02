package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type tuiScreen int

const (
	screenHome tuiScreen = iota
	screenLibrary
	screenConfirm
	screenPath
	screenProgress
	screenResult
	screenDeleteConfirm
	screenError
)

type tuiAction struct {
	id          string
	label, help string
}

type loginFinishedMsg struct{ err error }
type inspectionFinishedMsg struct {
	inspection LibraryInspection
	err        error
}
type verificationFinishedMsg struct {
	result VerifyResult
	err    error
}
type deletionFinishedMsg struct {
	result DeleteResult
	err    error
}
type openFinishedMsg struct{ err error }

type archiveMessage struct {
	event  *ArchiveEvent
	result *ArchiveResult
	err    error
}

type tuiModel struct {
	ctx         context.Context
	version     string
	demo        bool
	screen      tuiScreen
	width       int
	height      int
	dark        bool
	connected   bool
	archiveRoot string
	envPath     string
	archive     *Archive
	inspection  *LibraryInspection
	cursor      int
	busy        bool
	busyText    string
	status      string
	err         error
	spinner     spinner.Model
	progress    progress.Model
	pathInput   textinput.Model
	deleteInput textinput.Model
	cancel      context.CancelFunc
	events      <-chan archiveMessage
	archiveRun  ArchiveResult
	verifyRun   VerifyResult
	recent      []DownloadResult
	deletePlan  DeletePlan
	errorNote   string
}

func newTUIModel(ctx context.Context, version string, demo bool) tuiModel {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	bar := progress.New(
		progress.WithDefaultBlend(),
		progress.WithFillCharacters('━', '─'),
		progress.WithoutPercentage(),
	)
	input := textinput.New()
	input.Prompt = "Archive folder  "
	input.CharLimit = 1024
	input.SetValue(defaultArchiveRoot())
	input.CursorEnd()
	deleteInput := textinput.New()
	deleteInput.Prompt = "Type DELETE  "
	deleteInput.CharLimit = len("DELETE")
	archive, _ := NewArchive(defaultArchiveRoot())
	model := tuiModel{
		ctx:         ctx,
		version:     version,
		demo:        demo,
		screen:      screenHome,
		archiveRoot: defaultArchiveRoot(),
		envPath:     defaultEnvFile(),
		archive:     archive,
		spinner:     spin,
		progress:    bar,
		pathInput:   input,
		deleteInput: deleteInput,
	}
	if _, _, err := loadCredentials(model.envPath); err == nil {
		model.connected = true
	}
	if demo {
		model.connected = true
		model.archive = &Archive{Root: model.archiveRoot}
		inspection := demoLibraryInspection(model.archiveRoot)
		model.inspection = &inspection
		model.screen = screenLibrary
	}
	return model
}

func demoLibraryInspection(root string) LibraryInspection {
	return LibraryInspection{
		Total:          847,
		TotalBytes:     386 * 1024 * 1024 * 1024,
		Archived:       312,
		ArchivedBytes:  142 * 1024 * 1024 * 1024,
		Remaining:      531,
		RemainingBytes: 244 * 1024 * 1024 * 1024,
		Manual:         4,
		Earliest:       "2018-06-14",
		Latest:         "2026-07-28",
		Types:          map[string]int{"Photo": 493, "Video": 350, "MultiClipEdit": 4},
		ArchiveRoot:    root,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.spinner.Tick)
}

func (m tuiModel) homeActions() []tuiAction {
	actions := []tuiAction{}
	if !m.connected {
		actions = append(actions, tuiAction{"connect", "Connect GoPro", "Sign in safely on GoPro's website"})
	} else {
		actions = append(actions, tuiAction{"library", "View GoPro library", "Read-only — nothing downloads"})
	}
	if m.archive != nil && m.archive.Exists {
		actions = append(actions, tuiAction{"verify", "Check archive", "Read and verify every saved file"})
		if _, err := os.Stat(m.archive.ReportPath); err == nil {
			actions = append(actions, tuiAction{"report", "Open report", "View the archive report in your browser"})
		}
		actions = append(actions, tuiAction{"delete", "Delete local archive", "Remove its saved files from this computer"})
	}
	actions = append(actions, tuiAction{"quit", "Quit", "Leave everything as it is"})
	return actions
}

func (m *tuiModel) reloadArchive() {
	if m.demo {
		m.archive = &Archive{Root: m.archiveRoot}
		return
	}
	archive, err := NewArchive(m.archiveRoot)
	if err == nil {
		m.archive = archive
	}
}

func (m *tuiModel) startTask(label string) context.Context {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.busy = true
	m.busyText = label
	m.status = ""
	m.err = nil
	m.errorNote = ""
	return ctx
}

func loginCmd(ctx context.Context, envPath string) tea.Cmd {
	return func() tea.Msg { return loginFinishedMsg{err: connectAccountInBrowser(ctx, envPath)} }
}

func inspectCmd(ctx context.Context, root, envPath string) tea.Cmd {
	return func() tea.Msg {
		inspection, err := InspectLibrary(ctx, root, envPath, 100)
		return inspectionFinishedMsg{inspection: inspection, err: err}
	}
}

func verifyCmd(ctx context.Context, root string) tea.Cmd {
	return func() tea.Msg {
		result, err := VerifyArchive(ctx, root, "")
		return verificationFinishedMsg{result: result, err: err}
	}
}

func deleteCmd(ctx context.Context, root string) tea.Cmd {
	return func() tea.Msg {
		result, err := DeleteLocalArchive(ctx, root)
		return deletionFinishedMsg{result: result, err: err}
	}
}

func openCmd(path string) tea.Cmd {
	return func() tea.Msg { return openFinishedMsg{err: openURL("file://" + filepath.ToSlash(path))} }
}

func waitArchiveMessage(events <-chan archiveMessage) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func startArchive(ctx context.Context, options ArchiveOptions, demo bool) <-chan archiveMessage {
	events := make(chan archiveMessage, 64)
	go func() {
		defer close(events)
		if demo {
			start := time.Now()
			var transferred int64
			for index := 1; index <= 12; index++ {
				select {
				case <-ctx.Done():
					events <- archiveMessage{err: ctx.Err()}
					return
				case <-time.After(120 * time.Millisecond):
				}
				download := DownloadResult{MediaID: fmt.Sprintf("demo-%03d", index), Status: "ok", Bytes: int64(140+index*9) * 1024 * 1024}
				transferred += download.Bytes
				event := ArchiveEvent{Stage: "Archiving originals", Current: index, Total: 12, Transferred: transferred, Result: &download}
				events <- archiveMessage{event: &event}
			}
			result := ArchiveResult{Transferred: transferred, Elapsed: time.Since(start), Summary: Summary{Archived: 324, Files: 324, Bytes: 144 * 1024 * 1024 * 1024, Manual: 4}, Verification: VerificationResult{CheckedItems: 324, CheckedFiles: 324, CheckedBytes: 144 * 1024 * 1024 * 1024}}
			events <- archiveMessage{result: &result}
			return
		}
		result, err := ArchiveLibrary(ctx, options, func(event ArchiveEvent) {
			copy := event
			select {
			case events <- archiveMessage{event: &copy}:
			case <-ctx.Done():
			}
		})
		events <- archiveMessage{result: &result, err: err}
	}()
	return events
}

func (m *tuiModel) beginArchive() tea.Cmd {
	ctx := m.startTask("Preparing your archive")
	m.screen = screenProgress
	m.recent = nil
	m.archiveRun = ArchiveResult{}
	m.progress.SetPercent(0)
	m.events = startArchive(ctx, ArchiveOptions{
		Root:        m.archiveRoot,
		EnvPath:     m.envPath,
		LegacyState: defaultLegacyState(),
		Parallel:    8,
		PerPage:     100,
	}, m.demo)
	return waitArchiveMessage(m.events)
}

func (m *tuiModel) stopTask() {
	if m.cancel != nil {
		m.cancel()
	}
	m.status = "Stopping safely — finished files will be kept"
}

func normalizeArchivePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("archive folder cannot be empty")
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return filepath.Abs(value)
}

func (m tuiModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.pathInput.SetStyles(textinput.DefaultStyles(m.dark))
		m.deleteInput.SetStyles(textinput.DefaultStyles(m.dark))
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress.SetWidth(max(20, min(72, msg.Width-12)))
		m.pathInput.SetWidth(max(20, min(72, msg.Width-20)))
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(msg)
		commands = append(commands, command)
	case progress.FrameMsg:
		var command tea.Cmd
		m.progress, command = m.progress.Update(msg)
		commands = append(commands, command)
	case loginFinishedMsg:
		m.busy, m.cancel = false, nil
		if errors.Is(msg.err, context.Canceled) {
			m.screen = screenHome
			m.status = "Stopped"
		} else if msg.err != nil {
			m.err, m.screen = msg.err, screenError
		} else {
			m.connected = true
			m.status = "Connected to GoPro"
			m.cursor = 0
		}
	case inspectionFinishedMsg:
		m.busy, m.cancel = false, nil
		if errors.Is(msg.err, context.Canceled) {
			m.screen = screenHome
			m.status = "Stopped"
		} else if msg.err != nil {
			if errors.Is(msg.err, ErrAuth) {
				m.connected = false
			}
			m.err, m.screen = msg.err, screenError
		} else {
			m.inspection = &msg.inspection
			m.screen = screenLibrary
		}
	case verificationFinishedMsg:
		m.busy, m.cancel = false, nil
		m.verifyRun = msg.result
		if errors.Is(msg.err, context.Canceled) {
			m.screen = screenHome
			m.status = "Stopped"
		} else if msg.err != nil && !errors.Is(msg.err, ErrArchiveIncomplete) {
			m.err, m.screen = msg.err, screenError
		} else {
			m.screen = screenResult
			m.status = "Archive check complete"
			m.reloadArchive()
		}
	case deletionFinishedMsg:
		m.busy, m.cancel = false, nil
		if msg.err != nil {
			m.err, m.screen = msg.err, screenError
			m.errorNote = "Deletion did not finish. Some local files may already be gone. GoPro cloud media was not touched."
		} else {
			m.reloadArchive()
			m.inspection = nil
			m.screen, m.cursor = screenHome, 0
			m.status = fmt.Sprintf("Deleted %d local file(s) · %s. GoPro cloud media was not touched.", msg.result.RemovedFiles, humanBytes(msg.result.RemovedBytes))
		}
	case openFinishedMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.screen = msg.err, screenError
		} else {
			m.status = "Opened the archive report"
		}
	case archiveMessage:
		if msg.event != nil {
			m.busyText = msg.event.Stage
			if msg.event.Result != nil {
				m.recent = append(m.recent, *msg.event.Result)
				if len(m.recent) > 5 {
					m.recent = m.recent[len(m.recent)-5:]
				}
			}
			if msg.event.Total > 0 {
				command := m.progress.SetPercent(float64(msg.event.Current) / float64(msg.event.Total))
				commands = append(commands, command)
			}
			commands = append(commands, waitArchiveMessage(m.events))
		} else {
			m.busy, m.cancel = false, nil
			if msg.result != nil {
				m.archiveRun = *msg.result
			}
			if errors.Is(msg.err, context.Canceled) {
				m.screen, m.cursor = screenHome, 0
				m.status = "Stopped safely. Run archive again to continue."
				m.reloadArchive()
			} else if msg.err != nil && !errors.Is(msg.err, ErrArchiveIncomplete) {
				m.err, m.screen = msg.err, screenError
			} else {
				m.screen = screenResult
				m.status = "Archive run complete"
				m.reloadArchive()
			}
		}
	}

	key, isKey := message.(tea.KeyPressMsg)
	if !isKey {
		return m, tea.Batch(commands...)
	}
	stroke := key.String()
	if m.screen == screenPath {
		switch stroke {
		case "esc":
			m.pathInput.Blur()
			m.screen = screenLibrary
			return m, nil
		case "enter":
			root, err := normalizeArchivePath(m.pathInput.Value())
			if err != nil {
				m.pathInput.Err = err
				return m, nil
			}
			m.archiveRoot = root
			m.pathInput.Blur()
			if m.demo {
				inspection := demoLibraryInspection(root)
				m.inspection = &inspection
			} else if m.inspection != nil {
				inspection, planErr := ReplanLibrary(root, *m.inspection)
				if planErr != nil {
					m.err, m.screen = planErr, screenError
					return m, nil
				}
				m.inspection = &inspection
			}
			m.reloadArchive()
			m.screen = screenLibrary
			return m, nil
		}
		var command tea.Cmd
		m.pathInput, command = m.pathInput.Update(message)
		return m, command
	}
	if m.screen == screenDeleteConfirm && !m.busy {
		switch stroke {
		case "esc":
			m.deleteInput.Blur()
			m.deleteInput.Err = nil
			m.screen = screenHome
			return m, nil
		case "enter":
			if m.deleteInput.Value() != "DELETE" {
				m.deleteInput.Err = errors.New("type DELETE exactly to continue")
				return m, nil
			}
			m.deleteInput.Blur()
			ctx := m.startTask("Deleting the local archive — GoPro cloud untouched")
			return m, deleteCmd(ctx, m.archiveRoot)
		}
		var command tea.Cmd
		m.deleteInput, command = m.deleteInput.Update(message)
		return m, command
	}
	if stroke == "ctrl+c" || stroke == "q" {
		if m.busy {
			m.stopTask()
			return m, nil
		}
		return m, tea.Quit
	}
	if m.busy {
		if stroke == "esc" {
			m.stopTask()
		}
		return m, tea.Batch(commands...)
	}

	switch m.screen {
	case screenHome:
		actions := m.homeActions()
		switch stroke {
		case "up", "k":
			m.cursor = (m.cursor - 1 + len(actions)) % len(actions)
		case "down", "j":
			m.cursor = (m.cursor + 1) % len(actions)
		case "enter":
			action := actions[m.cursor].id
			switch action {
			case "connect":
				ctx := m.startTask("Waiting for you to sign in on GoPro")
				commands = append(commands, loginCmd(ctx, m.envPath))
			case "library":
				ctx := m.startTask("Reading your GoPro library — nothing will download")
				commands = append(commands, inspectCmd(ctx, m.archiveRoot, m.envPath))
			case "verify":
				ctx := m.startTask("Checking every archived file")
				commands = append(commands, verifyCmd(ctx, m.archiveRoot))
			case "report":
				m.busy, m.busyText = true, "Opening your archive report"
				commands = append(commands, openCmd(m.archive.ReportPath))
			case "delete":
				plan, err := PlanLocalArchive(m.archiveRoot)
				if err != nil {
					m.err, m.screen = err, screenError
					break
				}
				m.deletePlan = plan
				m.deleteInput.SetValue("")
				m.deleteInput.Err = nil
				m.deleteInput.Focus()
				m.screen = screenDeleteConfirm
			case "quit":
				commands = append(commands, tea.Quit)
			}
		}
	case screenLibrary:
		switch stroke {
		case "enter", "a":
			if m.inspection != nil {
				m.screen = screenConfirm
			}
		case "e":
			m.pathInput.SetValue(m.archiveRoot)
			m.pathInput.CursorEnd()
			m.pathInput.Focus()
			m.screen = screenPath
		case "esc", "b":
			m.screen, m.cursor = screenHome, 0
		}
	case screenConfirm:
		switch stroke {
		case "enter", "a":
			commands = append(commands, m.beginArchive())
		case "e":
			m.pathInput.SetValue(m.archiveRoot)
			m.pathInput.CursorEnd()
			m.pathInput.Focus()
			m.screen = screenPath
		case "esc", "b":
			m.screen = screenLibrary
		}
	case screenProgress:
		if stroke == "esc" {
			m.stopTask()
		}
	case screenResult:
		switch stroke {
		case "o":
			path := m.archiveRun.ReportPath
			if path == "" {
				path = m.verifyRun.ReportPath
			}
			if path == "" && m.archive != nil {
				path = m.archive.ReportPath
			}
			if path != "" {
				m.busy, m.busyText = true, "Opening your archive report"
				commands = append(commands, openCmd(path))
			}
		case "enter", "esc", "b":
			m.screen, m.cursor = screenHome, 0
		}
	case screenError:
		if stroke == "enter" || stroke == "esc" || stroke == "b" {
			m.err, m.errorNote, m.screen, m.cursor = nil, "", screenHome, 0
		}
	}
	return m, tea.Batch(commands...)
}

func (m tuiModel) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "GoPro Yank"
	return view
}

func runTUI(ctx context.Context, version string, demo bool) error {
	_, err := tea.NewProgram(newTUIModel(ctx, version, demo), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return exitError{130, ctx.Err()}
	}
	return err
}

func terminalIsInteractive() bool {
	input, inputErr := os.Stdin.Stat()
	output, outputErr := os.Stdout.Stat()
	return inputErr == nil && outputErr == nil && input.Mode()&os.ModeCharDevice != 0 && output.Mode()&os.ModeCharDevice != 0
}

func sortedTypeNames(types map[string]int) []string {
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
