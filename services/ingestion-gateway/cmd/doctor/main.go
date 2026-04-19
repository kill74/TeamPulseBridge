package main

import (
	"context"
	"errors"
	"fmt"
	goversion "go/version"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	report := &doctorReport{}

	fmt.Println("== TeamPulseBridge Doctor ==")

	root, err := findRepoRoot()
	if err != nil {
		report.failf("workspace: %v", err)
		report.finish()
		return
	}

	fmt.Println()
	fmt.Println("Tooling:")
	report.checkGoVersion()
	report.checkCommand("git", "Git", true)
	report.checkDocker()
	report.checkOptionalCommand("golangci-lint", "golangci-lint")
	report.checkOptionalCommand("python3", "python3")

	fmt.Println()
	fmt.Println("Workspace:")
	report.checkPreCommitHook(root)
	report.checkEnvFiles(root)

	report.finish()
}

type doctorReport struct {
	failures int
	warnings int
}

func (r *doctorReport) okf(format string, args ...any) {
	fmt.Printf("  [OK] "+format+"\n", args...)
}

func (r *doctorReport) warnf(format string, args ...any) {
	r.warnings++
	fmt.Printf("  [WARN] "+format+"\n", args...)
}

func (r *doctorReport) failf(format string, args ...any) {
	r.failures++
	fmt.Printf("  [FAIL] "+format+"\n", args...)
}

func (r *doctorReport) finish() {
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  failures: %d\n", r.failures)
	fmt.Printf("  warnings: %d\n", r.warnings)

	if r.failures > 0 {
		fmt.Println()
		fmt.Println("Doctor found blocking issues.")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Doctor checks passed.")
}

func (r *doctorReport) checkCommand(name, label string, required bool) bool {
	if _, err := exec.LookPath(name); err != nil {
		if required {
			r.failf("%s is missing", label)
		} else {
			r.warnf("%s is missing", label)
		}
		return false
	}
	r.okf("%s is installed", label)
	return true
}

func (r *doctorReport) checkOptionalCommand(name, label string) {
	_ = r.checkCommand(name, label, false)
}

func (r *doctorReport) checkGoVersion() {
	goVersion := runtime.Version()
	if strings.HasPrefix(goVersion, "devel") {
		r.warnf("Go version %q is a development build; expected stable >= go1.22", goVersion)
		return
	}
	if !goversion.IsValid(goVersion) {
		r.failf("unable to parse Go version %q", goVersion)
		return
	}
	if goversion.Compare(goVersion, "go1.22") >= 0 {
		r.okf("Go version %s is supported (>= go1.22)", goVersion)
		return
	}
	r.failf("Go version %s is too old; expected >= go1.22", goVersion)
}

func (r *doctorReport) checkDocker() {
	if !r.checkCommand("docker", "Docker CLI", true) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runCommand(ctx, "docker", "info"); err != nil {
		r.failf("Docker daemon is not reachable: %v", err)
	} else {
		r.okf("Docker daemon is reachable")
	}

	ctxCompose, cancelCompose := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCompose()
	if err := runCommand(ctxCompose, "docker", "compose", "version"); err != nil {
		r.failf("Docker Compose plugin is missing or unavailable: %v", err)
	} else {
		r.okf("Docker Compose plugin is available")
	}
}

func (r *doctorReport) checkPreCommitHook(root string) {
	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookPath); err == nil {
		r.okf("pre-commit hook is installed")
		return
	}
	r.warnf("pre-commit hook is not installed (run 'make dev-setup' or 'make precommit-install')")
}

func (r *doctorReport) checkEnvFiles(root string) {
	envPath := filepath.Join(root, ".env")
	examplePath := filepath.Join(root, ".env.example")

	envFile := ""
	switch {
	case fileExists(envPath):
		r.okf(".env file is present")
		envFile = envPath
	case fileExists(examplePath):
		r.warnf(".env file is missing (copy from .env.example for local development)")
		envFile = examplePath
	default:
		r.failf("neither .env nor .env.example is present")
		return
	}

	values, err := parseEnvFile(envFile)
	if err != nil {
		r.failf("unable to parse %s: %v", filepath.Base(envFile), err)
		return
	}

	queueBackend := strings.TrimSpace(values["QUEUE_BACKEND"])
	if queueBackend == "" {
		queueBackend = "log"
	}
	if queueBackend == "pubsub" {
		if strings.TrimSpace(values["PUBSUB_PROJECT_ID"]) == "" {
			r.failf("QUEUE_BACKEND=pubsub but PUBSUB_PROJECT_ID is empty in %s", filepath.Base(envFile))
		} else {
			r.okf("PUBSUB_PROJECT_ID is set for pubsub backend")
		}
		if strings.TrimSpace(values["PUBSUB_TOPIC_ID"]) == "" {
			r.failf("QUEUE_BACKEND=pubsub but PUBSUB_TOPIC_ID is empty in %s", filepath.Base(envFile))
		} else {
			r.okf("PUBSUB_TOPIC_ID is set for pubsub backend")
		}
		return
	}

	r.okf("QUEUE_BACKEND is %q", queueBackend)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for i := 0; i < 6; i++ {
		if fileExists(filepath.Join(dir, "Makefile")) && fileExists(filepath.Join(dir, ".env.example")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("unable to locate repository root from current directory")
}

func parseEnvFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
