package tui

import (
	"fmt"
	"gw2-cmd-watch/parser"
	"gw2-cmd-watch/processor"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Confirmation check takes priority
	if m.confirming {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				switch m.confirmationType {
				case confirmDeleteRun:
					cmds = append(cmds, deleteRun(m.itemToDelete))
					m.status = fmt.Sprintf("Deleting run: %s", filepath.Base(m.itemToDelete))
				case confirmDeleteLog:
					fullPath := m.logFullPaths[m.itemToDelete]
					cmds = append(cmds, deleteLogFiles(fullPath))
					// Optimistically remove from UI
					delete(m.logs, fullPath)
					delete(m.logFullPaths, m.itemToDelete)
					for i, name := range m.logList {
						if name == m.itemToDelete {
							m.logList = append(m.logList[:i], m.logList[i+1:]...)
							break
						}
					}
					if m.selectedIndex >= len(m.logList)+1 {
						m.selectedIndex = len(m.logList)
					}
					m.status = fmt.Sprintf("Deleted log: %s", m.itemToDelete)
				case confirmAppUpdate:
					cmds = append(cmds, openFile(m.updateURL))
					m.status = "Opening browser to download update..."
				}
				m.confirming = false
				m.itemToDelete = ""
				m.updateURL = ""
			case "n", "N", "esc":
				m.confirming = false
				m.itemToDelete = ""
				m.updateURL = ""
				m.status = "Action cancelled."
			}
		}
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.styles.RightPanel = m.styles.RightPanel.Width(m.width - m.styles.LeftPanel.GetWidth() - m.styles.LeftPanel.GetHorizontalFrameSize())
		m.styles.RightPanel = m.styles.RightPanel.Height(m.height - 5)
		return m, nil

	case UpdateAvailableMsg:
		m.confirming = true
		m.confirmationType = confirmAppUpdate
		m.updateURL = msg.URL
		m.status = "A new version is available! Open download page? (y/N)"
		return m, nil

	case RunsLoadedMsg:
		m.runList = msg.Runs
		m.status = fmt.Sprintf("Found %d archived runs.", len(m.runList))
		return m, nil

	case SingleLogParsedMsg:
		// Add the log to the model as it's parsed
		m.logs[msg.FullPath] = msg.Log
		displayName := strings.Replace(filepath.Base(msg.FullPath), "_detailed_wvw_kill.json", "", 1)
		m.logList = append(m.logList, displayName)
		m.logFullPaths[displayName] = msg.FullPath
		m.status = fmt.Sprintf("Loading... %d logs parsed.", len(m.logList))
		return m, nil

	case AllLogsParsedMsg:
		// Now that all logs are loaded, sort the list
		sort.Strings(m.logList)
		m.status = fmt.Sprintf("Loaded %d logs from run.", len(m.logList))
		if len(m.logList) > 0 {
			m.selectedIndex = 1 // Select the first log
		} else {
			m.selectedIndex = 0 // Select ../
		}
		return m, nil

	case TempLogProcessedMsg:
		// This is the entry point for a new, live log.
		// We parse it here to decide where it goes.
		parsedLog, err := parser.ParseLog(msg.TempPath)
		if err != nil {
			m.err = err
			return m, nil
		}

		var finalRunPath string
		isNewRun := m.viewMode == runsView || (m.viewMode == logsView && len(m.logList) >= 30)

		if isNewRun {
			m.viewMode = logsView
			m.clearCurrentRun()

			commander := "UnknownCommander"
			for _, p := range parsedLog.Players {
				if p.HasCommanderTag {
					commander = p.Account
					break
				}
			}
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			runName := fmt.Sprintf("%s_%s", commander, timestamp)
			finalRunPath = filepath.Join(processor.LogArchive, runName)
			m.currentRunPath = finalRunPath
			m.currentRunName = runName
			m.status = "New run started."
		} else {
			// Add to the currently viewed run
			finalRunPath = m.currentRunPath
		}
		return m, archiveLogFile(msg.TempPath, finalRunPath, parsedLog)

	case LogfileArchivedMsg:
		// This message confirms the file has been moved. Now we add it to the UI.
		// We only perform the auto-selection if the archived log belongs to the run we are currently viewing.
		archivedRunPath := filepath.Dir(msg.FullPath)
		if archivedRunPath == m.currentRunPath {
			m.logs[msg.FullPath] = msg.Log
			displayName := strings.Replace(filepath.Base(msg.FullPath), "_detailed_wvw_kill.json", "", 1)
			if _, exists := m.logFullPaths[displayName]; !exists {
				m.logList = append(m.logList, displayName)
			}
			m.logFullPaths[displayName] = msg.FullPath
			sort.Strings(m.logList)
			// Find the new index of the just-added log to select it
			for i, name := range m.logList {
				if name == displayName {
					m.selectedIndex = i + 1 // +1 for ../
					break
				}
			}
			m.selectedCard = 0
			m.status = fmt.Sprintf("New log processed: %s", displayName)
		}
		return m, nil

	case StatusMsg:
		m.status = string(msg)
	case ErrMsg:
		m.err = msg.Err
	case tea.KeyMsg:
		switch m.focusedPanel {
		case leftPanel:
			return m.handleLeftPanelKeys(msg)
		case rightPanel:
			return m.handleRightPanelKeys(msg)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m model) handleLeftPanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	currentListSize := m.getCurrentListSize()

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "w", "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}
	case "s", "down", "j":
		if m.selectedIndex < currentListSize-1 {
			m.selectedIndex++
		}
	case "d", "right", "l":
		m.focusedPanel = rightPanel
	case "ctrl+d":
		if m.viewMode == runsView && m.selectedIndex > 0 {
			runName := m.runList[m.selectedIndex-1]
			m.confirming = true
			m.confirmationType = confirmDeleteRun
			m.itemToDelete = filepath.Join(processor.LogArchive, runName)
			m.status = fmt.Sprintf("Delete run '%s'? (y/N)", runName)
		} else if m.viewMode == logsView && m.selectedIndex > 0 {
			logName := m.logList[m.selectedIndex-1]
			m.confirming = true
			m.confirmationType = confirmDeleteLog
			m.itemToDelete = logName
			m.status = fmt.Sprintf("Delete log '%s'? (y/N)", logName)
		}
	case "enter", " ":
		cmd = m.handleSelection()
	}
	return m, cmd
}

func (m model) handleRightPanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "a", "left", "h":
		m.focusedPanel = leftPanel
	case "w", "up", "k":
		if m.selectedCard > 0 {
			m.selectedCard--
		}
	case "s", "down", "j":
		if m.selectedCard < 8 {
			m.selectedCard++
		}
	case "enter", " ":
		if m.viewMode == logsView && m.selectedIndex > 0 {
			displayName := m.logList[m.selectedIndex-1]
			jsonFullPath := m.logFullPaths[displayName]
			htmlPath := strings.Replace(jsonFullPath, ".json", ".html", 1)
			return m, openFile(htmlPath)
		}
	}
	return m, nil
}

func (m *model) handleSelection() tea.Cmd {
	if m.viewMode == runsView {
		if m.selectedIndex == 0 { // "New Run"
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			runName := fmt.Sprintf("UnknownCommander_%s", timestamp)
			m.currentRunPath = filepath.Join(processor.LogArchive, runName)
			m.currentRunName = runName
			m.viewMode = logsView
			m.clearCurrentRun()
			m.status = "New run created. Waiting for logs."
			return func() tea.Msg {
				// Ensure the directory gets created on disk
				return os.MkdirAll(m.currentRunPath, 0755)
			}
		} else { // A run from the list
			runName := m.runList[m.selectedIndex-1]
			m.currentRunPath = filepath.Join(processor.LogArchive, runName)
			m.currentRunName = runName
			m.viewMode = logsView
			m.clearCurrentRun()
			m.status = fmt.Sprintf("Loading logs for run: %s", runName)
			return loadLogsInRun(m.currentRunPath)
		}
	} else { // logsView
		if m.selectedIndex == 0 { // "../"
			m.viewMode = runsView
			m.currentRunPath = ""
			m.currentRunName = "Viewing Run Archives"
			m.clearCurrentRun()
			m.selectedIndex = 0
			return loadRuns
		}
		// If in logsView, selection is handled by the right panel (shows data)
		m.selectedCard = 0
	}
	return nil
}

func (m *model) getCurrentListSize() int {
	if m.viewMode == runsView {
		return len(m.runList) + 1 // +1 for "New Run"
	}
	return len(m.logList) + 1 // +1 for "../"
}
