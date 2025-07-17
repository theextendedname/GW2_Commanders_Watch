package tui

import (
	"fmt"
	"gw2-cmd-watch/config"
	"gw2-cmd-watch/parser"
	"gw2-cmd-watch/processor"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/skratchdot/open-golang/open"
)

// --- Message Types ---
type TempLogProcessedMsg struct{ TempPath string } // From processor, contains path to temp JSON
type LogfileArchivedMsg struct {                   // From self, after file is moved
	Log      *parser.ParsedLog
	FullPath string
}
type ErrMsg struct{ Err error }
type StatusMsg string
type RunsLoadedMsg struct{ Runs []string }

// New messages for concurrent parsing
type SingleLogParsedMsg struct {
	Log      *parser.ParsedLog
	FullPath string
}
type AllLogsParsedMsg struct{}

type UpdateAvailableMsg struct{ URL string }

// --- TUI State Enums ---
type panel int
type logListViewMode int
type confirmationMode int

const (
	leftPanel panel = iota
	rightPanel
)

const (
	runsView logListViewMode = iota
	logsView
)

const (
	confirmDeleteRun confirmationMode = iota
	confirmDeleteLog
	confirmAppUpdate
)

// --- Model ---
type model struct {
	width  int
	height int
	theme  ShadesOfPurple
	styles Styles
	config config.Config

	// Data
	logs         map[string]*parser.ParsedLog // Map full path to parsed log
	runList      []string                     // List of directory names in Log_Archive
	logList      []string                     // List of file names in a selected run
	logFullPaths map[string]string            // Map filename to full path for the current run

	// State
	viewMode       logListViewMode
	currentRunPath string
	currentRunName string
	selectedIndex  int
	focusedPanel   panel
	selectedCard   int

	// Status
	status           string
	err              error
	confirming       bool
	confirmationType confirmationMode
	itemToDelete     string // Can be a run path or a log display name
	updateURL        string // URL for the new app version
}

func NewModel(cfg config.Config, initialRuns []string) model {
	theme := NewShadesOfPurple()
	return model{
		theme:          theme,
		styles:         NewStyles(theme),
		config:         cfg,
		status:         "Select a run or wait for a new one.",
		focusedPanel:   leftPanel,
		viewMode:       runsView,
		runList:        initialRuns,
		logs:           make(map[string]*parser.ParsedLog),
		logFullPaths:   make(map[string]string),
		currentRunName: "Viewing Run Archives",
	}
}

func (m model) Init() tea.Cmd {
	return loadRuns // Initial command to load runs
}

// --- Command Functions ---

func loadRuns() tea.Msg {
	var runs []string
	files, err := os.ReadDir(processor.LogArchive)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(processor.LogArchive, 0755)
			return StatusMsg("Log_Archive directory created.")
		}
		return ErrMsg{Err: err}
	}
	for _, file := range files {
		if file.IsDir() {
			runs = append(runs, file.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(runs))) // Sort newest first
	return RunsLoadedMsg{Runs: runs}
}

func loadLogsInRun(runPath string) tea.Cmd {
	return func() tea.Msg {
		files, err := os.ReadDir(runPath)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var cmds []tea.Cmd
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				fullPath := filepath.Join(runPath, file.Name())
				cmds = append(cmds, parseSingleLog(fullPath))
			}
		}
		return tea.Sequence(tea.Batch(cmds...), func() tea.Msg { return AllLogsParsedMsg{} })()
	}
}

func parseSingleLog(path string) tea.Cmd {
	return func() tea.Msg {
		parsedLog, err := parser.ParseLog(path)
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("failed to parse %s: %w", filepath.Base(path), err)}
		}
		return SingleLogParsedMsg{Log: parsedLog, FullPath: path}
	}
}

func archiveLogFile(tempJsonPath, finalRunPath string, log *parser.ParsedLog) tea.Cmd {
	return func() tea.Msg {
		archivedPath, err := processor.ArchiveLogFiles(tempJsonPath, finalRunPath)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return LogfileArchivedMsg{Log: log, FullPath: archivedPath}
	}
}

func deleteRun(path string) tea.Cmd {
	return func() tea.Msg {
		if err := os.RemoveAll(path); err != nil {
			return ErrMsg{Err: fmt.Errorf("failed to delete run: %w", err)}
		}
		return loadRuns()
	}
}

func deleteLogFiles(jsonPath string) tea.Cmd {
	return func() tea.Msg {
		htmlPath := strings.Replace(jsonPath, ".json", ".html", 1)
		if err := os.Remove(jsonPath); err != nil {
			fmt.Printf("Warning: failed to delete JSON file %s: %v\n", jsonPath, err)
		}
		if err := os.Remove(htmlPath); err != nil {
			fmt.Printf("Warning: failed to delete HTML file %s: %v\n", htmlPath, err)
		}
		return nil // Fire and forget, no message needed on success
	}
}

func (m *model) clearCurrentRun() {
	m.logs = make(map[string]*parser.ParsedLog)
	m.logList = []string{}
	m.logFullPaths = make(map[string]string)
	m.selectedIndex = 0
	m.selectedCard = 0
}

// --- View Functions ---

func (m model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}
	if m.confirming {
		return m.renderConfirmationView()
	}

	if m.focusedPanel == leftPanel {
		m.styles.LeftPanel = m.styles.LeftPanel.BorderForeground(m.theme.AccentCyan)
		m.styles.RightPanel = m.styles.RightPanel.BorderForeground(m.theme.Gray)
	} else {
		m.styles.LeftPanel = m.styles.LeftPanel.BorderForeground(m.theme.Gray)
		m.styles.RightPanel = m.styles.RightPanel.BorderForeground(m.theme.AccentCyan)
	}

	left := m.renderLeftPanel()
	right := m.renderRightPanel()
	statusBar := m.renderStatusBar()
	helpBar := m.renderHelpBar()

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, mainContent, statusBar, helpBar)
}

func (m *model) renderConfirmationView() string {
	// The confirmation question is already set in the model's status field.
	return m.styles.ConfirmationPrompt.Render(m.status)
}

func (m *model) renderLeftPanel() string {
	var items []string
	if m.viewMode == logsView {
		items = append(items, "../")
	} else {
		items = append(items, "New Run")
	}

	switch m.viewMode {
	case runsView:
		items = append(items, m.runList...)
	case logsView:
		items = append(items, m.logList...)
	}

	var content strings.Builder
	title := m.currentRunName
	if m.viewMode == logsView {
		parts := strings.SplitN(m.currentRunName, "_", 2)
		if len(parts) == 2 {
			commanderName := strings.Split(parts[0], ".")[0]
			title = commanderName + "\n" + parts[1]
		}
	}
	content.WriteString(m.styles.CardTitle.Render(title) + "\n\n")

	for i, item := range items {
		style := m.styles.ListItem
		prefix := "  "
		if i == m.selectedIndex {
			style = m.styles.SelectedListItem
			prefix = "> "
		}

		if m.viewMode == runsView && i >= 1 {
			parts := strings.SplitN(item, "_", 2)
			if len(parts) == 2 {
				commanderName := strings.Split(parts[0], ".")[0]
				var commanderNameStyle lipgloss.Style
				if i == m.selectedIndex {
					commanderNameStyle = lipgloss.NewStyle().Foreground(m.theme.AccentYellowAlt).Bold(true)
				} else {
					commanderNameStyle = lipgloss.NewStyle().Foreground(m.theme.AccentOrange)
				}
				content.WriteString(style.Render(prefix))
				content.WriteString(commanderNameStyle.Render(commanderName))
				content.WriteString("\n")
				line2 := "  " + parts[1]
				content.WriteString(style.Render(line2))
				content.WriteString("\n")
			} else {
				content.WriteString(style.Render(prefix+item) + "\n")
			}
		} else {
			content.WriteString(style.Render(prefix+item) + "\n")
		}
	}
	return m.styles.LeftPanel.Render(content.String())
}

func (m *model) renderRightPanel() string {
	var selectedLog *parser.ParsedLog
	if m.viewMode == logsView && m.selectedIndex > 0 && m.selectedIndex <= len(m.logList) {
		displayName := m.logList[m.selectedIndex-1]
		fullPath := m.logFullPaths[displayName]
		selectedLog = m.logs[fullPath]
	}

	if selectedLog == nil {
		dashText := `GW2 Commanders Watch - Report Dashboard

No log selected.
A new run is created or added to when a new log is detected in your arcDPS log folder.

Quick Guide

Move: Use WASD, JK, or Up/Down Arrows.
D / Right Arrow: Go to Report Dashboard.
A / Left Arrow: Go back to Log List.
W/S / Up/Down Arrow: Move selection up and down.
Select: Press Enter or Spacebar.
Delete: Ctrl+D for Archives/Logs.
Zoom: Ctrl+Plus/Minus (requires Windows Terminal).
Quit: Ctrl+C or Q.

Important Notes

arcDPS Logs: Default location is C:\Users\<USERNAME>\Documents\Guild Wars 2\addons\arcdps\arcdps.cbtlogs.
App Data: GW2 Commanders Watch stores data in Log_Archive next to the executable.
Detailed Reports: Press D (Report Dashboard), then Enter or Spacebar to open a log in your browser.
Parser: This app uses the Gw2 Elite Insights Parser (https://github.com/baaron4/GW2-Elite-Insights-Parser).
Feedback/Support for GW2 Commanders Watch: https://github.com/theextendedname

`
		return m.styles.RightPanel.Render(dashText)
	}

	bannerCard := m.buildBannerInfoCard(selectedLog)
	summaryCard := m.buildSummaryCard(selectedLog)
	damageCard := m.buildDamageCard(selectedLog)
	downContribCard := m.buildDownContributionCard(selectedLog)
	cleansesCard := m.buildCleansesCard(selectedLog)
	stripsCard := m.buildStripsCard(selectedLog)
	healingCard := m.buildHealingCard(selectedLog)
	barrierCard := m.buildBarrierCard(selectedLog)
	deathCard := m.buildDeathCard(selectedLog)

	cardContents := map[int]string{0: summaryCard, 1: bannerCard, 2: damageCard, 3: downContribCard, 4: cleansesCard, 5: stripsCard, 6: deathCard, 7: healingCard, 8: barrierCard}
	for i, content := range cardContents {
		style := m.styles.Card
		if m.focusedPanel == rightPanel && i == m.selectedCard {
			style = m.styles.SelectedCard
		}
		cardContents[i] = style.Render(content)
	}

	row1 := lipgloss.JoinHorizontal(lipgloss.Top, cardContents[0], cardContents[1])
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, cardContents[2], cardContents[3])
	row3 := lipgloss.JoinHorizontal(lipgloss.Top, cardContents[4], cardContents[5], cardContents[6])
	row4 := lipgloss.JoinHorizontal(lipgloss.Top, cardContents[7], cardContents[8])
	finalLayout := lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3, row4)
	return m.styles.RightPanel.Render(finalLayout)
}

func (m *model) renderStatusBar() string {
	var statusText string
	if m.err != nil {
		statusText = m.styles.ErrorText.Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		statusText = m.status
	}
	w := lipgloss.Width
	statusWidth := w(statusText)
	versionInfo := "v0.1.0"
	versionWidth := w(versionInfo)
	padding := m.width - statusWidth - versionWidth - m.styles.StatusBar.GetHorizontalFrameSize()
	if padding < 0 {
		padding = 0
	}
	return m.styles.StatusBar.Render(lipgloss.JoinHorizontal(lipgloss.Top, statusText, strings.Repeat(" ", padding), versionInfo))
}

func (m *model) renderHelpBar() string {
	helpLine1 := "WSAD/Arrows: Navigate • Enter/Space: Select • q: Quit"
	var helpLine2 string
	if m.viewMode == logsView {
		helpLine2 = "ctrl+d: Delete Log • ctrl+plus/minus: Zoom"
	} else {
		helpLine2 = "ctrl+d: Delete Run • ctrl+plus/minus: Zoom"
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.styles.HelpBar.Render(helpLine1), m.styles.HelpBar.Render(helpLine2))
}

// formatNumber adds comma separators to an integer.
func formatNumber(n int) string {
	in := strconv.Itoa(n)
	out := make([]byte, len(in)+(len(in)-1)/3)
	if n < 0 {
		in = in[1:]
	}
	for i, j, k := len(in)-1, len(out)-1, 0; ; i, j = i-1, j-1 {
		out[j] = in[i]
		if i == 0 {
			if n < 0 {
				return "-" + string(out)
			}
			return string(out)
		}
		if k++; k == 3 {
			j, k = j-1, 0
			out[j] = ','
		}
	}
}

// Card Builder Functions
// Point represents a 2D coordinate
type Point struct {
	X float64
	Y float64
}

// CalculateDistance calculates the Euclidean distance between two Point objects.
func CalculateDistance(p1, p2 Point) float64 {
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y
	return math.Sqrt(dx*dx+dy*dy) * 100 // Scale to match GW2 units
}

func (m *model) buildBannerInfoCard(log *parser.ParsedLog) string {
	var location string
	switch {
	case strings.HasPrefix(log.FightName, "Detailed WvW - Blue"):
		location = "BBL"
	case strings.HasPrefix(log.FightName, "Detailed WvW - Red"):
		location = "RBL"
	case strings.HasPrefix(log.FightName, "Detailed WvW - Green"):
		location = "GBL"
	case strings.HasPrefix(log.FightName, "Detailed WvW - Eternal"):
		location = "EBG"
	default:
		location = "PvE"
	}
	var startTime string
	parts := strings.Split(log.TimeStart, " ")
	if len(parts) > 1 {
		startTime = parts[1]
	}
	var sb strings.Builder
	sb.WriteString(m.styles.CardTitle.Render(fmt.Sprintf("%-9s %-14s %s", "Location", "Duration", "Fight Start")) + "\n")
	sb.WriteString(fmt.Sprintf("%-9s %-14s %s", location, log.Duration, startTime))
	return sb.String()
}

func (m *model) buildSummaryCard(log *parser.ParsedLog) string {
	var squadDmg, squadDps, squadDowns, squadDeaths, enemyCount, enemyDmg, enemyDps, enemyDowns, enemyDeaths int
	var inSquadCount, notInSquadCount, zergCount int
	for _, p := range log.Players {
		if p.NotInSquad {
			notInSquadCount++
		} else {
			inSquadCount++
			if len(p.DpsTargets) > 0 {
				for _, dpsT := range p.DpsTargets {
					for _, dpsTarget := range dpsT {
						squadDps += dpsTarget.Dps
						squadDmg += dpsTarget.Damage
					}
				}
			}
			if len(p.Defenses) > 0 {
				squadDeaths += p.Defenses[0].DeadCount
				squadDowns += p.Defenses[0].DownCount
			}
			if len(p.StatsTargets) > 0 {
				// Count downs and deaths for enemy players
				// use StatsTargets
				//this is the correct way to do it, don't change it
				for _, ST := range p.StatsTargets {
					for _, stAry := range ST {
						enemyDowns += stAry.Downed
						enemyDeaths += stAry.Killed
					}
				}
			}
		}
	}

	zergCount = inSquadCount + notInSquadCount
	for _, t := range log.Targets {
		if t.EnemyPlayer && !t.IsFakeTarget {
			enemyCount++
			if len(t.StatsAll) > 0 {
				enemyDmg += t.StatsAll[0].Dmg
			}
			if len(t.DpsAll) > 0 {
				enemyDps += t.DpsAll[0].Dps
			}
		}
	}
	var sb strings.Builder
	rowStr := fmt.Sprintf("%-15s %-12s %-8s %-5s %s ", "Fight Balance", "DMG", "DPS", "Downs", "Deaths")
	sb.WriteString(m.styles.CardTitle.Render(rowStr) + "\n")
	sb.WriteString(fmt.Sprintf("Squad %-2d(%-2d/%-2d) %-12s %-8s %-5s %s", zergCount, inSquadCount, notInSquadCount, formatNumber(squadDmg), formatNumber(squadDps), formatNumber(squadDowns), formatNumber(squadDeaths)) + "\n")
	sb.WriteString(fmt.Sprintf("Enemy %-9d %-12s %-8s %-5s %s", enemyCount, formatNumber(enemyDmg), formatNumber(enemyDps), formatNumber(enemyDowns), formatNumber(enemyDeaths)))
	return sb.String()
}

func (m *model) buildDamageCard(log *parser.ParsedLog) string {
	type playerDamage struct {
		name   string
		damage int
		dps    int
	}
	var players []playerDamage
	for _, p := range log.Players {
		if p.NotInSquad {
			continue
		}
		var totalDmg, totalDps int
		for _, dpsT := range p.DpsTargets {
			for _, dpsTarget := range dpsT {
				totalDmg += dpsTarget.Damage
				totalDps += dpsTarget.Dps
			}
		}
		players = append(players, playerDamage{name: p.Name, damage: totalDmg, dps: totalDps})
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].damage > players[j].damage
	})
	var sb strings.Builder
	sb.WriteString(m.styles.CardTitle.Render(fmt.Sprintf("%-20s %-10s %s", "Damage Top 5", "T-DMG", "DPS")) + "\n")
	for i, p := range players {
		if i >= 5 {
			break
		}
		rowStr := fmt.Sprintf("%-20s %-10s %s", p.name, formatNumber(p.damage), formatNumber(p.dps))
		if i%2 != 0 {
			sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
		} else {
			sb.WriteString(rowStr + "\n")
		}
	}
	return sb.String()
}

func (m *model) buildDownContributionCard(log *parser.ParsedLog) string {
	type playerDowns struct {
		name    string
		downCon int
		downs   int
	}
	var players []playerDowns
	for _, p := range log.Players {
		if p.NotInSquad {
			continue
		}
		var totalDownCon, totalDowns int
		for _, st := range p.StatsTargets {
			for _, statTarget := range st {
				totalDownCon += statTarget.DownContribution
				totalDowns += statTarget.Downed
			}
		}
		if totalDownCon > 0 {
			players = append(players, playerDowns{name: p.Name, downCon: totalDownCon, downs: totalDowns})
		}
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].downCon > players[j].downCon
	})
	var sb strings.Builder
	sb.WriteString(m.styles.CardTitle.Render(fmt.Sprintf("%-20s %-10s %s", "Downs Top 5", "Down-Cont", "Downs")) + "\n")
	for i, p := range players {
		if i >= 5 {
			break
		}
		rowStr := fmt.Sprintf("%-20s %-10s %s", p.name, formatNumber(p.downCon), formatNumber(p.downs))
		if i%2 != 0 {
			sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
		} else {
			sb.WriteString(rowStr + "\n")
		}
	}
	return sb.String()
}

// Refactored buildCleansesCard function
func (m *model) buildCleansesCard(log *parser.ParsedLog) string {
	var players []parser.Player
	for _, p := range log.Players {
		if !p.NotInSquad {
			players = append(players, p)
		}
	}

	sort.Slice(players, func(i, j int) bool {
		// Calculate totalCondiCleanse for player i
		totalCondiCleanseI := 0
		if len(players[i].Support) > 0 {
			totalCondiCleanseI = players[i].Support[0].CondiCleanse + players[i].Support[0].CondiCleanseSelf
		}

		// Calculate totalCondiCleanse for player j
		totalCondiCleanseJ := 0
		if len(players[j].Support) > 0 {
			totalCondiCleanseJ = players[j].Support[0].CondiCleanse + players[j].Support[0].CondiCleanseSelf
		}

		// Sort in descending order (highest totalCondiCleanse first)
		return totalCondiCleanseI > totalCondiCleanseJ
	})

	var sb strings.Builder
	sb.WriteString(m.styles.CardTitle.Render("Cleanses") + "\n")

	for i, p := range players {
		if i >= 5 {
			break
		}

		playerCondiCleanseSelf := 0
		playerCondiCleanse := 0
		if len(p.Support) > 0 {
			playerCondiCleanseSelf = p.Support[0].CondiCleanseSelf
			playerCondiCleanse = p.Support[0].CondiCleanse
		}
		totalCondiCleanse := playerCondiCleanse + playerCondiCleanseSelf

		if totalCondiCleanse > 0 { // Only display if totalCondiCleanse is greater than 0
			rowStr := fmt.Sprintf("%-20s %s", p.Name, formatNumber(totalCondiCleanse))
			if i%2 != 0 {
				sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
			} else {
				sb.WriteString(rowStr + "\n")
			}
		}
	}
	return sb.String()
}

func (m *model) buildStripsCard(log *parser.ParsedLog) string {
	var players []parser.Player
	for _, p := range log.Players {
		if !p.NotInSquad {
			players = append(players, p)
		}
	}
	sort.Slice(players, func(i, j int) bool {
		if len(players[i].Support) == 0 || len(players[j].Support) == 0 {
			return false
		}
		return players[i].Support[0].BoonStrips > players[j].Support[0].BoonStrips
	})
	var sb strings.Builder
	sb.WriteString(m.styles.CardTitle.Render("Boon Strips") + "\n")
	for i, p := range players {
		if i >= 5 {
			break
		}
		if len(p.Support) > 0 && p.Support[0].BoonStrips > 0 {
			rowStr := fmt.Sprintf("%-20s %s", p.Name, formatNumber(p.Support[0].BoonStrips))
			if i%2 != 0 {
				sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
			} else {
				sb.WriteString(rowStr + "\n")
			}
		}
	}
	return sb.String()
}

func (m *model) buildDeathCard(log *parser.ParsedLog) string {
	type playerDeath struct {
		name       string
		deathTime  float64 // Use a float for sorting, with a max value for N/A
		distToCmd  float64
		incomingCC int
	}
	var deadPlayers []playerDeath

	// Find the commander
	var commander *parser.Player
	for i := range log.Players {
		if log.Players[i].HasCommanderTag {
			commander = &log.Players[i]
			break
		}
	}

	pollingRate := log.CombatReplayMetaData.PollingRate

	for _, p := range log.Players {
		if !p.NotInSquad && len(p.Defenses) > 0 && p.Defenses[0].DeadCount > 0 {
			var deathTimeValue float64 = math.MaxFloat64 // Default for sorting
			if len(p.CombatReplayData.Dead) > 0 && len(p.CombatReplayData.Dead[0]) > 1 {
				if deathTime, ok := p.CombatReplayData.Dead[0][0].(float64); ok {
					deathTimeValue = deathTime
				}
			}

			distToCmd := -1.0 // Default distance if calculation fails
			if commander != nil && pollingRate > 0 && deathTimeValue != math.MaxFloat64 {
				timeIndex := int(math.Round(deathTimeValue / float64(pollingRate)))

				if timeIndex >= 0 && timeIndex < len(p.CombatReplayData.Positions) && timeIndex < len(commander.CombatReplayData.Positions) {
					playerPosData := p.CombatReplayData.Positions[timeIndex]
					cmdrPosData := commander.CombatReplayData.Positions[timeIndex]

					if len(playerPosData) >= 2 && len(cmdrPosData) >= 2 {
						playerPoint := Point{X: playerPosData[0], Y: playerPosData[1]}
						cmdrPoint := Point{X: cmdrPosData[0], Y: cmdrPosData[1]}
						distToCmd = CalculateDistance(playerPoint, cmdrPoint)
					}
				}
			}
			// Fallback to old value if calculation failed
			if distToCmd == -1.0 || p.HasCommanderTag {
				distToCmd = float64(p.StatsAll[0].DistToCommander)
			}

			deadPlayers = append(deadPlayers, playerDeath{
				name:       p.Name,
				deathTime:  deathTimeValue,
				distToCmd:  distToCmd,
				incomingCC: p.Defenses[0].ReceivedCrowdControl,
			})
		}
	}

	// Sort by the death time; players with actual times will appear first.
	sort.Slice(deadPlayers, func(i, j int) bool {
		return deadPlayers[i].deathTime < deadPlayers[j].deathTime
	})

	var sb strings.Builder
	title := fmt.Sprintf("%-20s %-11s %-12s %s", "First 5 To Die", "Time(H:m:s)", "DistToTag", "CC")
	sb.WriteString(m.styles.CardTitle.Render(title) + "\n")

	for i, p := range deadPlayers {
		if i >= 5 {
			break
		}

		var timeStr string
		var rowStr string
		if p.deathTime < math.MaxFloat64 {
			duration := time.Duration(p.deathTime) * time.Millisecond
			hours := int(duration.Hours())
			minutes := int(duration.Minutes()) % 60
			seconds := int(duration.Seconds()) % 60
			timeStr = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
		} else {
			timeStr = "N/A"
			continue // Skip this player if no valid death time
		}

		distStr := "N/A"
		if p.distToCmd >= 0 {
			distStr = fmt.Sprintf("%.2f", p.distToCmd)
		}

		rowStr = fmt.Sprintf("%-20s %-11s %-12s %d", p.name, timeStr, distStr, p.incomingCC)

		if i%2 != 0 {
			sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
		} else {
			sb.WriteString(rowStr + "\n")
		}
	}
	return sb.String()
}

// Refactored buildHealingCard function
func (m *model) buildHealingCard(log *parser.ParsedLog) string {
	type PlayerHealingData struct {
		Name         string
		TotalHealing int
		TotalHPS     int
	}
	var playerHealingReports []PlayerHealingData

	// Iterate through each player in the log to calculate their total healing and HPS.
	for _, p := range log.Players {
		// Only include players who are part of the squad.
		if !p.NotInSquad {
			totalHealing := 0
			totalHPS := 0

			// Loop through the multi-dimensional 'OutgoingHealingAllies' slice.
			// The outer loop iterates over each inner slice (e.g., each source of healing data).
			for _, healingSlice := range p.ExtHealingStats.OutgoingHealingAllies {
				// The inner loop iterates over each 'Healing' struct within the current inner slice.
				for _, healingData := range healingSlice {
					totalHealing += healingData.Healing
					totalHPS += healingData.Hps
				}
			}

			// Append the aggregated data to our report slice.
			playerHealingReports = append(playerHealingReports, PlayerHealingData{
				Name:         p.Name,
				TotalHealing: totalHealing,
				TotalHPS:     totalHPS,
			})
		}
	}

	// Sort the 'playerHealingReports' slice by 'TotalHealing' in descending order.
	// Players with higher total healing will appear first.
	sort.Slice(playerHealingReports, func(i, j int) bool {
		return playerHealingReports[i].TotalHealing > playerHealingReports[j].TotalHealing
	})

	var sb strings.Builder // Use a strings.Builder for efficient string concatenation.

	// Render the card title with appropriate formatting.
	headerStr := fmt.Sprintf("%-20s %-10s %s ", "Healing Top 5", "Healing", "HPS")
	sb.WriteString(m.styles.CardTitle.Render(headerStr) + "\n")

	// Iterate through the sorted players and build the report rows.
	for i, report := range playerHealingReports {
		// Limit the report to the top 5 players.
		if i >= 5 {
			break
		}

		// Only display players who have contributed some healing or HPS.
		if report.TotalHealing > 0 || report.TotalHPS > 0 {
			rowStr := fmt.Sprintf("%-20s %-10s %s", report.Name, formatNumber(report.TotalHealing), formatNumber(report.TotalHPS))

			// Apply alternating row styling for better readability.
			if i%2 != 0 {
				sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
			} else {
				sb.WriteString(rowStr + "\n")
			}
		}
	}
	return sb.String()
}

func (m *model) buildBarrierCard(log *parser.ParsedLog) string {
	var players []parser.Player
	for _, p := range log.Players {
		if !p.NotInSquad {
			players = append(players, p)
		}
	}
	sort.Slice(players, func(i, j int) bool {
		if len(players[i].ExtBarrierStats.OutgoingBarrier) == 0 || len(players[j].ExtBarrierStats.OutgoingBarrier) == 0 {
			return false
		}
		return players[i].ExtBarrierStats.OutgoingBarrier[0].Barrier > players[j].ExtBarrierStats.OutgoingBarrier[0].Barrier
	})
	var sb strings.Builder
	rowStr := fmt.Sprintf("%-20s %-10s %s ", "Barrier Top 5", "Barrier", "BPS")
	sb.WriteString(m.styles.CardTitle.Render(rowStr) + "\n")
	for i, p := range players {
		if i >= 5 {
			break
		}
		if len(p.ExtBarrierStats.OutgoingBarrier) > 0 {
			rowStr := fmt.Sprintf("%-20s %-10s %s", p.Name, formatNumber(p.ExtBarrierStats.OutgoingBarrier[0].Barrier), formatNumber(p.ExtBarrierStats.OutgoingBarrier[0].Bps))
			if i%2 != 0 {
				sb.WriteString(lipgloss.NewStyle().Background(m.theme.AccentDarkPurple).Foreground(m.theme.Foreground).Render(rowStr) + "\n")
			} else {
				sb.WriteString(rowStr + "\n")
			}
		}
	}
	return sb.String()
}

type Styles struct {
	LeftPanel          lipgloss.Style
	RightPanel         lipgloss.Style
	StatusBar          lipgloss.Style
	HelpBar            lipgloss.Style
	ListItem           lipgloss.Style
	SelectedListItem   lipgloss.Style
	ErrorText          lipgloss.Style
	Card               lipgloss.Style
	SelectedCard       lipgloss.Style
	CardTitle          lipgloss.Style
	ConfirmationPrompt lipgloss.Style
}

func NewStyles(theme ShadesOfPurple) Styles {
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Gray).Padding(0, 0).Margin(0, 0, 0, 0)
	return Styles{
		LeftPanel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Gray).Padding(0, 0).Width(23),
		RightPanel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Gray).Padding(0, 0),
		Card: cardStyle,
		SelectedCard: cardStyle.Copy().
			Border(lipgloss.ThickBorder()).
			BorderForeground(theme.AccentCyan),
		CardTitle: lipgloss.NewStyle().
			Bold(true).Foreground(theme.AccentYellow),
		StatusBar: lipgloss.NewStyle().
			Foreground(theme.Foreground).Background(theme.AccentDarkPurple).Padding(0, 1),
		HelpBar: lipgloss.NewStyle().
			Foreground(theme.Gray).Padding(0, 1),
		ListItem: lipgloss.NewStyle().
			Padding(0, 0, 0, 0),
		SelectedListItem: lipgloss.NewStyle().
			Foreground(theme.AccentCyan).Bold(true),
		ErrorText: lipgloss.NewStyle().
			Foreground(theme.AccentRed),
		ConfirmationPrompt: lipgloss.NewStyle().
			Background(theme.AccentGreen).
			Foreground(theme.Background).
			Padding(0, 1),
	}
}

func openFile(path string) tea.Cmd {
	return func() tea.Msg {
		err := open.Run(path)
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("could not open file: %w", err)}
		}
		return StatusMsg(fmt.Sprintf("Opening report: %s", path))
	}
}
