package main

import (
	"bufio"
	"fmt"
	"gw2-cmd-watch/config"
	"gw2-cmd-watch/eicli"
	"gw2-cmd-watch/processor"
	"gw2-cmd-watch/tui"
	"gw2-cmd-watch/updater"
	"gw2-cmd-watch/watcher"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func main() {

	logFile, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer logFile.Close()
	if runtime.GOOS == "windows" {
		// For cmd.exe and PowerShell, you can use the 'title' command.
		// Note: This launches a new process, so error handling is important.

		cmd := exec.Command("cmd", "/C", "title", "GW2_Commanders_Watch")
		err := cmd.Run()
		if err != nil {
			fmt.Println("Error setting console title:", err)
		}
	}
	const configPath = "config.json"
	cfg, err := loadOrInitConfig(configPath)
	if err != nil {
		fmt.Printf("Error with configuration: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Using WatchFolder: %s\n", cfg.WatchFolder)

	// Ensure the Elite Insights config file exists
	ensureEICLIConfig()

	// Clean up the temp folder from any previous runs
	if err := os.RemoveAll(processor.FightLogTemp); err != nil {
		fmt.Printf("Warning: could not clear temp folder: %v\n", err)
	}
	if err := os.MkdirAll(processor.FightLogTemp, 0755); err != nil {
		fmt.Printf("Warning: could not recreate temp folder: %v\n", err)
	}

	// Get initial list of runs
	initialRuns, err := getInitialRuns()
	if err != nil {
		fmt.Printf("Could not load initial runs: %v\n", err)
		// Don't exit, just start with an empty list
	}

	// Initialize the TUI program
	initialModel := tui.NewModel(cfg, initialRuns)
	p := tea.NewProgram(initialModel, tea.WithAltScreen())

	// Goroutine for App Updater
	go func() {
		updateInfo, err := updater.CheckForUpdates()
		if err != nil {
			// Don't bother the user, just log it
			fmt.Fprintf(logFile, "error checking for app update: %v\n", err)
		}
		if updateInfo != nil {
			p.Send(tui.UpdateAvailableMsg{URL: updateInfo.URL})
		}
	}()

	// Goroutine for CLI Auto-Updater
	cliUpdateChan := make(chan string)
	go eicli.InstallCLI(cliUpdateChan)
	go func() {
		for status := range cliUpdateChan {
			p.Send(tui.StatusMsg(status))
		}
	}()

	// Goroutine for File System Watcher
	fileEventChan := make(chan string)
	go func() {
		// Wait until the CLI is installed before starting the watcher
		for {
			if eicli.CheckCLIExists() {
				break
			}
			// This loop will be blocked by the InstallCLI goroutine's messages.
			// A more robust solution would use a dedicated channel, but this is sufficient.
			<-time.After(1 * time.Second)
		}
		if err := watcher.Start(cfg.WatchFolder, fileEventChan); err != nil {
			p.Send(tui.ErrMsg{Err: fmt.Errorf("watcher error: %w", err)})
		}
	}()

	// Goroutine for Log Processor
	go func() {
		for filePath := range fileEventChan {
			p.Send(tui.StatusMsg(fmt.Sprintf("Processing: %s", filepath.Base(filePath))))
			tempJSONPath, err := processor.ProcessLog(filePath)
			if err != nil {
				p.Send(tui.ErrMsg{Err: err})
			} else {
				p.Send(tui.TempLogProcessedMsg{TempPath: tempJSONPath})
			}
		}
	}()

	// Run the TUI
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}

func getInitialRuns() ([]string, error) {
	var runs []string
	files, err := os.ReadDir(processor.LogArchive)
	if err != nil {
		if os.IsNotExist(err) {
			return runs, nil // No archive yet, which is fine
		}
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() {
			runs = append(runs, file.Name())
		}
	}
	sort.Strings(runs)
	return runs, nil
}

func loadOrInitConfig(configPath string) (config.Config, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil || cfg.WatchFolder == "" {
		return promptForConfig(configPath)
	}
	if _, err := os.Stat(cfg.WatchFolder); os.IsNotExist(err) {
		fmt.Printf("Warning: WatchFolder '%s' from config.json does not exist.\n", cfg.WatchFolder)
		return promptForConfig(configPath)
	}
	return cfg, nil
}

func promptForConfig(configPath string) (config.Config, error) {
	var cfg config.Config
	reader := bufio.NewReader(os.Stdin)

	// Try to find a default path
	var defaultPath string
	homeDir, err := os.UserHomeDir()
	if err == nil {
		potentialPath := filepath.Join(homeDir, "Documents", "Guild Wars 2", "addons", "arcdps", "arcdps.cbtlogs")
		if _, err := os.Stat(potentialPath); err == nil {
			defaultPath = potentialPath
		}
	}
	if defaultPath == "" {
		// run CLI fallback
		cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", "$HOME")
		output, err := cmd.CombinedOutput()
		// Check stdout for default path
		if output != nil && err == nil {
			defaultPath = filepath.Join(strings.TrimRight(string(output), "\r\n"), "Documents", "Guild Wars 2", "addons", "arcdps", "arcdps.cbtlogs")
		}
	}
	for {
		if defaultPath != "" {
			baseStyle := lipgloss.NewStyle().Background(lipgloss.Color("#A5FF90")).Foreground(lipgloss.Color("#2d2b57")).Padding(0, 1)
			highlightStyle := lipgloss.NewStyle().Background(lipgloss.Color("#B362FF")).Foreground(lipgloss.Color("#2d2b57")).Padding(0, 1)

			fmt.Print(baseStyle.Render("Enter path for ArcDPS logs or"))
			fmt.Print(highlightStyle.Render("press Enter"))
			fmt.Print(baseStyle.Render("to use default:"))
			fmt.Printf("\n(%s): ", defaultPath)

		} else {

			// If no default path, just prompt normally
			baseStyle := lipgloss.NewStyle().Background(lipgloss.Color("#A5FF90")).Foreground(lipgloss.Color("#2d2b57")).Padding(0, 1)
			fmt.Print(baseStyle.Render("Default location is (C:\\Users\\<USERNAME>\\Documents\\Guild Wars 2\\addons\\arcdps\\arcdps.cbtlogs)"))
			fmt.Print(baseStyle.Render("Enter the absolute path for your ArcDPS log folder (WatchFolder):"))

		}

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// If user presses Enter and a default exists, use it
		if input == "" && defaultPath != "" {
			input = defaultPath
		}

		if input == "" {
			fmt.Println("Path cannot be empty.")
			continue
		}

		absPath, err := filepath.Abs(input)
		if err != nil {
			fmt.Printf("Error: Invalid path. %v\n", err)
			continue
		}

		fileInfo, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				style := lipgloss.NewStyle().Background(lipgloss.Color("#A5FF90")).Foreground(lipgloss.Color("#2d2b57")).Padding(0, 1)
				fmt.Print(style.Render(fmt.Sprintf("Folder '%s' does not exist. Create it? (y/N): ", absPath)))
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) == "y" {
					if err := os.MkdirAll(absPath, 0755); err != nil {
						fmt.Printf("Error creating folder: %v\n", err)
						continue
					}
					fmt.Printf("Folder '%s' created.\n", absPath)
					cfg.WatchFolder = absPath
					break
				}
			} else {
				fmt.Printf("Error checking folder: %v\n", err)
			}
			continue
		}

		if !fileInfo.IsDir() {
			fmt.Printf("Error: '%s' is a file, not a folder.\n", absPath)
			continue
		}

		cfg.WatchFolder = absPath
		break
	}

	if err := config.SaveConfig(configPath, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to save configuration: %w", err)
	}

	return cfg, nil
}

func ensureEICLIConfig() {
	const eiConfigPath = "ELI3.conf"
	const defaultConfig = `LightTheme=False
HtmlExternalScripts=False
SaveOutHTML=True
HtmlExternalScriptsPath=
CompressRaw=False
SaveOutCSV=False
IndentJSON=False
ParseMultipleLogs=False
AutoAddPath=
HtmlExternalScriptsCdn=
Outdated=False
OutLocation=.\FightLogTemp
AutoAdd=False
SendSimpleMessageToWebhook=False
RawTimelineArrays=True
UploadToRaidar=False
SaveOutJSON=True
PopulateHourLimit=0
SingleThreaded=False
SkipFailedTries=False
SaveOutXML=False
ParseCombatReplay=True
IndentXML=False
CustomTooShort=2200
AutoDiscordBatch=False
ApplicationTraces=False
Anonymous=False
WebhookURL=
AddPoVProf=False
UploadToWingman=False
AddDuration=False
HtmlCompressJson=False
AutoParse=False
SaveAtOut=False
DetailledWvW=True
SaveOutTrace=True
UploadToDPSReports=False
ComputeDamageModifiers=True
DPSReportUserToken=
SendEmbedToWebhook=False
MemoryLimit=0
ParsePhases=True`

	if _, err := os.Stat(eiConfigPath); os.IsNotExist(err) {
		fmt.Printf("'%s' not found. Creating with default settings...\n", eiConfigPath)
		if err := os.WriteFile(eiConfigPath, []byte(defaultConfig), 0644); err != nil {
			fmt.Printf("Error: Failed to create '%s': %v\n", eiConfigPath, err)
		}
	}
}
