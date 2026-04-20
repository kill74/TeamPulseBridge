param(
    [switch]$SkipSmoke,
    [switch]$SkipPolicy,
    [switch]$SkipTerraform,
    [switch]$SkipRace
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$serviceRoot = Join-Path $repoRoot "services\ingestion-gateway"
$terraformRoot = Join-Path $repoRoot "infrastructure\terraform"

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host "== $Message =="
}

function Invoke-NativeCommand([string]$FilePath, [string[]]$Arguments) {
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        $renderedArgs = if ($Arguments.Count -gt 0) { " $($Arguments -join ' ')" } else { "" }
        throw "Command failed with exit code ${LASTEXITCODE}: $FilePath$renderedArgs"
    }
}

function Resolve-CommandPath([string[]]$Candidates, [string]$InstallHint) {
    foreach ($candidate in $Candidates) {
        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if ($command) {
            return $command.Source
        }
    }

    if ($Candidates -contains "golangci-lint" -or $Candidates -contains "govulncheck") {
        $goBin = Join-Path ((& go env GOPATH).Trim()) "bin"
        foreach ($candidate in $Candidates) {
            $exe = Join-Path $goBin "$candidate.exe"
            if (Test-Path $exe) {
                return $exe
            }
        }
    }

    if ($Candidates -contains "checkov" -or $Candidates -contains "checkov.cmd") {
        $pythonRoots = Get-ChildItem (Join-Path $env:APPDATA "Python") -Directory -ErrorAction SilentlyContinue | Sort-Object Name -Descending
        foreach ($pythonRoot in $pythonRoots) {
            foreach ($candidate in @("checkov.cmd", "checkov")) {
                $match = Join-Path $pythonRoot.FullName "Scripts\$candidate"
                if (Test-Path $match) {
                    return $match
                }
            }
        }
    }

    throw $InstallHint
}

function Invoke-Checked([string]$FilePath, [string[]]$Arguments, [string]$WorkingDirectory) {
    Push-Location $WorkingDirectory
    try {
        Invoke-NativeCommand $FilePath $Arguments
    }
    finally {
        Pop-Location
    }
}

function Invoke-GoCheck([string[]]$Arguments) {
    Push-Location $serviceRoot
    try {
        Invoke-NativeCommand "go" $Arguments
    }
    finally {
        Pop-Location
    }
}

function Invoke-TerraformCheck([string[]]$Arguments, [hashtable]$Environment = @{}) {
    Push-Location $terraformRoot
    try {
        foreach ($entry in $Environment.GetEnumerator()) {
            Set-Item -Path "Env:$($entry.Key)" -Value $entry.Value
        }
        Invoke-NativeCommand $terraformExe $Arguments
    }
    finally {
        foreach ($entry in $Environment.GetEnumerator()) {
            Remove-Item -Path "Env:$($entry.Key)" -ErrorAction SilentlyContinue
        }
        Pop-Location
    }
}

$golangciLintExe = Resolve-CommandPath @("golangci-lint") "golangci-lint was not found. Run 'make dev-setup' or 'go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8'."
$govulncheckExe = Resolve-CommandPath @("govulncheck") "govulncheck was not found. Run 'make dev-setup' or 'go install golang.org/x/vuln/cmd/govulncheck@latest'."
$terraformExe = if (-not $SkipTerraform) { Resolve-CommandPath @("terraform") "terraform was not found. Install Terraform to run local CI parity." } else { $null }
$checkovExe = if (-not $SkipPolicy) { Resolve-CommandPath @("checkov", "checkov.cmd") "checkov was not found. Run 'make dev-setup' or install checkov==3.2.469." } else { $null }

Write-Step "Go fmt check"
Push-Location $serviceRoot
try {
    $unformatted = @(& gofmt -l ./cmd ./internal | Where-Object { $_ -and $_.Trim() -ne "" })
    if ($LASTEXITCODE -ne 0) {
        throw "gofmt failed while checking the repository."
    }
    if ($unformatted.Count -gt 0) {
        throw "Go files need formatting:`n$($unformatted -join [Environment]::NewLine)"
    }
}
finally {
    Pop-Location
}

Write-Step "Go vet"
Invoke-GoCheck @("vet", "./...")

Write-Step "Go test"
Invoke-GoCheck @("test", "./...")

Write-Step "golangci-lint"
Invoke-Checked $golangciLintExe @("run", "--config", "../../.golangci.yml", "./...") $serviceRoot

Write-Step "govulncheck"
Push-Location $serviceRoot
try {
    & $govulncheckExe -format text ./...
    if ($LASTEXITCODE -eq 3) {
        throw "govulncheck found reachable vulnerabilities. If the report points at the Go standard library, upgrade Go to the latest patch release and rerun."
    }
    if ($LASTEXITCODE -ne 0) {
        throw "govulncheck failed with exit code ${LASTEXITCODE}."
    }
}
finally {
    Pop-Location
}

if (-not $SkipRace) {
    Write-Step "Race tests"
    $cgoEnabled = ((& go env CGO_ENABLED) | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to determine CGO_ENABLED from 'go env'."
    }
    if ($cgoEnabled -ne "1") {
        throw "Race tests require CGO_ENABLED=1. On Windows this usually means installing a C toolchain. Re-run with -SkipRace for a partial local pass, or enable cgo for full CI parity."
    }
    Invoke-GoCheck @("test", "-race", "./...")
}

if (-not $SkipTerraform) {
    Write-Step "Terraform fmt/init/validate"
    $tfDataDir = Join-Path $terraformRoot ".terraform.ci-local"
    try {
        Invoke-TerraformCheck @("fmt", "-check", "-recursive")
        Invoke-TerraformCheck @("init", "-backend=false") @{ TF_DATA_DIR = $tfDataDir }
        Invoke-TerraformCheck @("validate") @{ TF_DATA_DIR = $tfDataDir }
    }
    finally {
        Remove-Item -LiteralPath $tfDataDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

if (-not $SkipPolicy) {
    Write-Step "Checkov policy checks"
    Invoke-Checked $checkovExe @("--config-file", ".checkov.yaml", "--framework", "terraform", "--directory", "infrastructure/terraform") $repoRoot
    Invoke-Checked $checkovExe @("--config-file", ".checkov.yaml", "--framework", "kubernetes", "--directory", "deploy/k8s", "--directory", "deploy/gitops/argocd") $repoRoot
}

if (-not $SkipSmoke) {
    Write-Step "Docker compose smoke"
    Push-Location $repoRoot
    try {
        try {
            Invoke-NativeCommand "docker" @("compose", "down", "-v")
        }
        catch {
        }

        Invoke-NativeCommand "docker" @("compose", "up", "-d", "--build")

        $ready = $false
        for ($i = 0; $i -lt 40; $i++) {
            try {
                $response = Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/healthz
                if ($response.StatusCode -eq 200) {
                    $ready = $true
                    break
                }
            }
            catch {
            }
            Start-Sleep -Seconds 3
        }

        if (-not $ready) {
            Invoke-NativeCommand "docker" @("compose", "ps")
            Invoke-NativeCommand "docker" @("compose", "logs", "--no-color", "--tail=200")
            throw "healthz did not become ready in time"
        }

        $metrics = Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/metrics
        if ($metrics.StatusCode -ne 200) {
            throw "/metrics returned status $($metrics.StatusCode)"
        }

        ($metrics.Content -split "`n" | Select-Object -First 20) | Out-Host
        Invoke-NativeCommand "docker" @("compose", "ps")
    }
    finally {
        try {
            Invoke-NativeCommand "docker" @("compose", "down", "-v")
        }
        catch {
        }
        Pop-Location
    }
}

Write-Host ""
Write-Host "Local CI parity checks completed successfully."
