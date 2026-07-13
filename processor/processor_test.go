package processor

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestEliteInsightsCommandOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific command")
	}

	homeDir := t.TempDir()
	binDir := filepath.Join(homeDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(homeDir, ".wine-dotnet8"), 0755); err != nil {
		t.Fatal(err)
	}
	winePath := filepath.Join(binDir, "wine")
	if err := os.WriteFile(winePath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("PATH", binDir)
	t.Setenv("WINEDEBUG", "old-value")
	t.Setenv("WINEPREFIX", "/old/prefix")

	cmd, err := eliteInsightsCommand("parser.exe", "ELI3.conf", "fight log.zevtc")
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{winePath, "parser.exe", "-c", "ELI3.conf", "fight log.zevtc"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("command args = %q, want %q", cmd.Args, wantArgs)
	}
	wantEnv := []string{
		"WINEDEBUG=-all",
		"WINEPREFIX=" + filepath.Join(homeDir, ".wine-dotnet8"),
		"DOTNET_gcServer=1",
	}
	for _, variable := range wantEnv {
		if !slices.Contains(cmd.Env, variable) {
			t.Errorf("command environment does not contain %q", variable)
		}
	}
	if slices.Contains(cmd.Env, "WINEDEBUG=old-value") || slices.Contains(cmd.Env, "WINEPREFIX=/old/prefix") {
		t.Error("command environment retained overridden Wine settings")
	}
}

func TestFilterWineFixme(t *testing.T) {
	input := []byte("normal output\n0024:fixme:kernelbase:AppPolicyGetProcessTerminationMethod stub\r\nmore output\n")
	want := "normal output\nmore output\n"

	if got := string(filterWineFixme(input)); got != want {
		t.Fatalf("filterWineFixme() = %q, want %q", got, want)
	}
}

func TestFilterWineFixmePreservesOutputWithoutTrailingNewline(t *testing.T) {
	input := []byte("normal output\nfixme:noise\nfinal output")
	want := "normal output\nfinal output"

	if got := string(filterWineFixme(input)); got != want {
		t.Fatalf("filterWineFixme() = %q, want %q", got, want)
	}
}
