package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type tuiStyles struct {
	title, tagline, section, primary, muted     lipgloss.Style
	selected, action, good, warning, bad, panel lipgloss.Style
}

func newTUIStyles(dark bool) tuiStyles {
	choose := lipgloss.LightDark(dark)
	bone := choose(lipgloss.Color("#182126"), lipgloss.Color("#F4F0E8"))
	cloud := choose(lipgloss.Color("#536872"), lipgloss.Color("#8FA7B2"))
	rule := choose(lipgloss.Color("#B7C4CA"), lipgloss.Color("#30424B"))
	yank := lipgloss.Color("#FF5C35")
	verified := choose(lipgloss.Color("#087F5B"), lipgloss.Color("#58E0B4"))
	amber := choose(lipgloss.Color("#9A6700"), lipgloss.Color("#F4BF50"))
	red := choose(lipgloss.Color("#B42318"), lipgloss.Color("#FF7B72"))
	return tuiStyles{
		title:    lipgloss.NewStyle().Bold(true).Foreground(bone),
		tagline:  lipgloss.NewStyle().Foreground(cloud),
		section:  lipgloss.NewStyle().Bold(true).Foreground(cloud),
		primary:  lipgloss.NewStyle().Foreground(bone),
		muted:    lipgloss.NewStyle().Foreground(cloud),
		selected: lipgloss.NewStyle().Bold(true).Foreground(yank),
		action:   lipgloss.NewStyle().Foreground(yank),
		good:     lipgloss.NewStyle().Foreground(verified),
		warning:  lipgloss.NewStyle().Foreground(amber),
		bad:      lipgloss.NewStyle().Foreground(red),
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(rule).Padding(1, 2),
	}
}

func (m tuiModel) render() string {
	styles := newTUIStyles(m.dark)
	width := m.width
	if width == 0 {
		width = 80
	}
	contentWidth := max(42, min(86, width-6))
	header := styles.title.Render("↓  GOPRO YANK") + styles.muted.Render("  v"+m.version) + "\n" +
		styles.tagline.Render("Bring your GoPro library home—and know nothing is missing.")
	body := ""
	switch m.screen {
	case screenHome:
		body = m.renderHome(styles)
	case screenLibrary:
		body = m.renderLibrary(styles)
	case screenConfirm:
		body = m.renderConfirm(styles)
	case screenPath:
		body = m.renderPath(styles)
	case screenProgress:
		body = m.renderProgress(styles)
	case screenResult:
		body = m.renderResult(styles)
	case screenError:
		body = m.renderError(styles)
	}
	if m.busy && m.screen != screenProgress {
		body += "\n\n" + styles.action.Render(m.spinner.View()+" "+m.busyText)
	}
	if m.status != "" && m.screen == screenHome {
		body += "\n\n" + styles.good.Render("✓ "+m.status)
	}
	content := header + "\n\n" + styles.panel.Width(contentWidth).Render(body)
	footer := ""
	if m.screen == screenHome && !m.busy {
		footer = "↑/↓ move   enter choose   q quit"
	} else if m.screen == screenPath {
		footer = "enter save folder   esc cancel"
	} else if m.busy {
		footer = "esc / q stop safely"
	} else {
		footer = "enter continue   esc back   q quit"
	}
	return lipgloss.NewStyle().Margin(1, 2).Render(content + "\n\n" + styles.muted.Render(footer))
}

func (m tuiModel) renderHome(styles tuiStyles) string {
	account := styles.warning.Render("Not connected")
	if m.connected {
		account = styles.good.Render("✓ Connected on this computer")
	}
	archive := styles.muted.Render("No archive yet")
	if m.archive != nil && m.archive.Exists {
		summary := m.archive.Summary()
		archive = styles.primary.Render(fmt.Sprintf("%d originals · %s saved", summary.Archived, humanBytes(summary.Bytes)))
		if summary.Blockers > 0 {
			archive += styles.warning.Render(fmt.Sprintf(" · %d need attention", summary.Blockers))
		}
	}
	sections := styles.section.Render("ACCOUNT") + "\n" + account + "\n\n" +
		styles.section.Render("ARCHIVE") + "\n" + archive + "\n" + styles.muted.Render(m.archiveRoot) + "\n\n" +
		styles.section.Render("WHAT WOULD YOU LIKE TO DO?")
	lines := []string{sections}
	for index, action := range m.homeActions() {
		pointer := "  "
		label := styles.primary.Render(action.label)
		if index == m.cursor {
			pointer = styles.selected.Render("› ")
			label = styles.selected.Render(action.label)
		}
		lines = append(lines, fmt.Sprintf("%s%-28s %s", pointer, label, styles.muted.Render(action.help)))
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderLibrary(styles tuiStyles) string {
	if m.inspection == nil {
		return styles.warning.Render("Library information is not available yet.")
	}
	i := *m.inspection
	dateRange := ""
	if i.Earliest != "" {
		dateRange = fmt.Sprintf(" · %s to %s", i.Earliest, i.Latest)
	}
	lines := []string{
		styles.section.Render("GOPRO LIBRARY"),
		styles.primary.Render(fmt.Sprintf("%d originals · %s%s", i.Total, humanBytes(i.TotalBytes), dateRange)),
		"",
	}
	for _, name := range sortedTypeNames(i.Types) {
		lines = append(lines, fmt.Sprintf("%-22s %6d", name, i.Types[name]))
	}
	lines = append(lines,
		"",
		styles.section.Render("ARCHIVE PLAN"),
		fmt.Sprintf("Already archived       %6d", i.Archived),
		fmt.Sprintf("Ready to archive       %6d  %s", i.Remaining, humanBytes(i.RemainingBytes)),
		fmt.Sprintf("Need manual export     %6d", i.Manual),
		styles.muted.Render(m.archiveRoot),
		"",
		styles.good.Render("Nothing was downloaded."),
		"",
		styles.action.Render("enter")+" archive this library    "+styles.action.Render("e")+" change folder    "+styles.muted.Render("esc back"),
	)
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderConfirm(styles tuiStyles) string {
	if m.inspection == nil {
		return styles.warning.Render("View the library before starting an archive.")
	}
	i := *m.inspection
	available := "unknown"
	if free, ok := freeDiskBytes(m.archiveRoot); ok {
		available = humanBytes(free)
	}
	lines := []string{
		styles.section.Render("START ARCHIVING?"),
		"",
		fmt.Sprintf("Originals to archive    %d", i.Remaining),
		fmt.Sprintf("Download size           %s", humanBytes(i.RemainingBytes)),
		fmt.Sprintf("Space available         %s", available),
		fmt.Sprintf("Folder                  %s", m.archiveRoot),
		"",
		styles.primary.Render("GoPro Yank will download and verify every available original."),
		styles.good.Render("It never deletes cloud or archived media."),
		"",
		styles.action.Render("enter") + " start archiving    " + styles.action.Render("e") + " change folder    " + styles.muted.Render("esc cancel"),
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderPath(styles tuiStyles) string {
	message := styles.section.Render("WHERE SHOULD THE ARCHIVE LIVE?") + "\n\n" + m.pathInput.View()
	if m.pathInput.Err != nil {
		message += "\n\n" + styles.bad.Render(m.pathInput.Err.Error())
	}
	message += "\n\n" + styles.muted.Render("Use a local folder or mounted external drive.")
	return message
}

func (m tuiModel) renderProgress(styles tuiStyles) string {
	lines := []string{
		styles.section.Render("ARCHIVING"),
		styles.action.Render(m.spinner.View() + " " + m.busyText),
		"",
		m.progress.View(),
	}
	if len(m.recent) > 0 {
		lines = append(lines, "", styles.section.Render("RECENT"))
		for _, result := range m.recent {
			if result.Status == "ok" {
				lines = append(lines, styles.good.Render("✓")+" "+result.MediaID+"  "+humanBytes(result.Bytes))
			} else {
				lines = append(lines, styles.bad.Render("✗")+" "+result.MediaID+"  "+result.Info)
			}
		}
	}
	if m.status != "" {
		lines = append(lines, "", styles.warning.Render(m.status))
	} else {
		lines = append(lines, "", styles.muted.Render("You can stop safely and continue later."))
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderResult(styles tuiStyles) string {
	summary := m.archiveRun.Summary
	verification := m.archiveRun.Verification
	if summary == (Summary{}) {
		summary = m.verifyRun.Summary
		verification = m.verifyRun.Verification
	}
	verdict := styles.good.Render("DOWNLOADABLE MEDIA EXPORT COMPLETE")
	if summary.Blockers > 0 || !verification.OK() {
		verdict = styles.warning.Render("MEDIA EXPORT NEEDS ATTENTION")
	}
	lines := []string{
		styles.section.Render("RESULT"),
		verdict,
		"",
		fmt.Sprintf("Archived originals      %d", summary.Archived),
		fmt.Sprintf("Saved files             %d", summary.Files),
		fmt.Sprintf("Saved                   %s", humanBytes(summary.Bytes)),
		fmt.Sprintf("Need manual export      %d", summary.Manual),
		fmt.Sprintf("Need attention          %d", summary.Blockers),
		"",
		verificationLine(styles, verification),
		"",
		styles.action.Render("o") + " open report    " + styles.muted.Render("enter home"),
	}
	return strings.Join(lines, "\n")
}

func verificationLine(styles tuiStyles, verification VerificationResult) string {
	line := fmt.Sprintf("Checked %d files · %s", verification.CheckedFiles, humanBytes(verification.CheckedBytes))
	if verification.OK() {
		return styles.good.Render(line)
	}
	return styles.warning.Render(fmt.Sprintf("%s · %d issue(s)", line, len(verification.Issues)))
}

func (m tuiModel) renderError(styles tuiStyles) string {
	message := "Something went wrong."
	if m.err != nil {
		message = m.err.Error()
	}
	return styles.section.Render("NEEDS ATTENTION") + "\n\n" + styles.bad.Render(message) + "\n\n" +
		styles.muted.Render("Nothing was deleted. Press enter to return home.")
}
