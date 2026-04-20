package main

import (
	"context"
	"errors"
	"fmt"
	goversion "go/version"
	"net"
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
	for _, tool := range []struct {
		name  string
		label string
	}{
		{name: "make", label: "Make"},
		{name: "golangci-lint", label: "golangci-lint"},
		{name: "python3", label: "python3"},
		{name: "govulncheck", label: "govulncheck"},
		{name: "terraform", label: "Terraform"},
		{name: "checkov", label: "checkov"},
		{name: "kubectl", label: "kubectl"},
		{name: "curl", label: "curl"},
	} {
		report.checkCommand(tool.name, tool.label, false)
	}

	fmt.Println()
	fmt.Println("Workspace:")
	report.checkPreCommitHook(root)
	report.checkEnvFiles(root)

	fmt.Println()
	fmt.Println("Local Ports:")
	report.checkCommonLocalPorts()

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

	r.checkTimedCommand(5*time.Second, "Docker daemon is reachable", "Docker daemon is not reachable: %v", "docker", "info")
	r.checkTimedCommand(5*time.Second, "Docker Compose plugin is available", "Docker Compose plugin is missing or unavailable: %v", "docker", "compose", "version")
}

func (r *doctorReport) checkTimedCommand(timeout time.Duration, okMessage, failFormat, name string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := runCommand(ctx, name, args...); err != nil {
		r.failf(failFormat, err)
		return
	}
	r.okf(okMessage)
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
		r.warnf(".env file is missing (run 'make env-init' to create it from .env.example)")
		envFile = examplePath
	default:
		r.failf("neither .env nor .env.example is present")
		return
	}
	envName := filepath.Base(envFile)

	values, err := parseEnvFile(envFile)
	if err != nil {
		r.failf("unable to parse %s: %v", envName, err)
		return
	}

	queueBackend := strings.TrimSpace(values["QUEUE_BACKEND"])
	if queueBackend == "" {
		queueBackend = "log"
	}
	if queueBackend == "pubsub" {
		r.checkRequiredEnvValues(envName, values, "QUEUE_BACKEND=pubsub but %s is empty in %s", "PUBSUB_PROJECT_ID", "PUBSUB_TOPIC_ID")
		return
	}

	r.okf("QUEUE_BACKEND is %q", queueBackend)

	if isTruthy(values["REQUIRE_SECRETS"]) {
		r.checkRequiredEnvValues(envName, values, "%s is required but empty in %s", "SLACK_SIGNING_SECRET", "GITHUB_WEBHOOK_SECRET", "GITLAB_WEBHOOK_TOKEN", "TEAMS_CLIENT_STATE")
	}

	if isTruthy(values["ADMIN_AUTH_ENABLED"]) {
		r.checkRequiredEnvValues(envName, values, "%s is required but empty in %s", "ADMIN_JWT_ISSUER", "ADMIN_JWT_AUDIENCE")
		secret := strings.TrimSpace(values["ADMIN_JWT_SECRET"])
		switch {
		case secret == "":
			r.failf("ADMIN_AUTH_ENABLED=true but ADMIN_JWT_SECRET is empty in %s", envName)
		case secret == "change-me" || len(secret) < 32:
			r.failf("ADMIN_AUTH_ENABLED=true but ADMIN_JWT_SECRET is weak in %s", envName)
		default:
			r.okf("ADMIN_JWT_SECRET looks configured for admin auth")
		}
	}
}

func (r *doctorReport) checkRequiredEnvValues(envName string, values map[string]string, failFormat string, keys ...string) {
	for _, key := range keys {
		if strings.TrimSpace(values[key]) == "" {
			r.failf(failFormat, key, envName)
			continue
		}
		r.okf("%s is set", key)
	}
}

func (r *doctorReport) checkCommonLocalPorts() {
	for _, port := range []struct {
		number int
		label  string
	}{
		{number: 8080, label: "Gateway / smoke checks"},
		{number: 9090, label: "Prometheus"},
		{number: 3000, label: "Grafana"},
		{number: 8085, label: "Pub/Sub emulator"},
	} {
		r.checkPortAvailable(port.number, port.label)
	}
}

func (r *doctorReport) checkPortAvailable(port int, label string) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		r.warnf("%s port %d is already in use; local stacks or CI parity checks may fail", label, port)
		return
	}
	_ = ln.Close()
	r.okf("%s port %d is available", label, port)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
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

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}
