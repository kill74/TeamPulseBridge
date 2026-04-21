param(
    [switch]$SkipHealthCheck,
    [switch]$NoBuild,
    [switch]$DryRun,
    [ValidateRange(1, 3600)]
    [int]$HealthTimeoutSeconds = 90
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$envPath = Join-Path $repoRoot ".env"
$examplePath = Join-Path $repoRoot ".env.example"
$healthUrl = "http://127.0.0.1:8080/healthz"

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host "== $Message =="
}

function Resolve-CommandPath([string]$Name, [string]$InstallHint) {
    if ($DryRun) {
        return $Name
    }

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) {
        throw $InstallHint
    }
    return $command.Source
}

function Format-Arguments([string[]]$Arguments) {
    if (-not $Arguments -or $Arguments.Count -eq 0) {
        return ""
    }

    $rendered = foreach ($argument in $Arguments) {
        if ($argument -match '\s') {
            '"' + $argument.Replace('"', '\"') + '"'
        }
        else {
            $argument
        }
    }
    return ($rendered -join " ")
}

function Invoke-NativeCommand([string]$FilePath, [string[]]$Arguments, [string]$WorkingDirectory = $repoRoot) {
    $rendered = Format-Arguments $Arguments
    if ($DryRun) {
        if ([string]::IsNullOrWhiteSpace($rendered)) {
            Write-Host "[dry-run] (cd $WorkingDirectory && $FilePath)"
        }
        else {
            Write-Host "[dry-run] (cd $WorkingDirectory && $FilePath $rendered)"
        }
        return
    }

    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "Command failed with exit code ${LASTEXITCODE}: $FilePath $rendered"
        }
    }
    finally {
        Pop-Location
    }
}

function Test-DockerPrerequisites() {
    Write-Step "Checking prerequisites"

    if ($DryRun) {
        Write-Host "[dry-run] would verify docker CLI, docker compose, and Docker daemon availability"
        return
    }

    $dockerExe = Resolve-CommandPath "docker" "docker was not found. Install Docker Desktop first."
    Push-Location $repoRoot
    try {
        try {
            & $dockerExe compose version *> $null
        }
        catch {
            throw "docker compose is not available. Install Docker Compose v2."
        }
        if ($LASTEXITCODE -ne 0) {
            throw "docker compose is not available. Install Docker Compose v2."
        }

        try {
            & $dockerExe info *> $null
        }
        catch {
            throw "Docker daemon is not reachable. Start Docker Desktop and retry."
        }
        if ($LASTEXITCODE -ne 0) {
            throw "Docker daemon is not reachable. Start Docker Desktop and retry."
        }
    }
    finally {
        Pop-Location
    }
}

function Initialize-LocalEnv() {
    Write-Step "Preparing local environment"

    if (Test-Path $envPath) {
        Write-Host ".env already exists; keeping your current local settings."
        return
    }

    if (-not (Test-Path $examplePath)) {
        throw ".env.example was not found in the repository root."
    }

    if ($DryRun) {
        Write-Host "[dry-run] would copy $examplePath to $envPath"
        return
    }

    Copy-Item -LiteralPath $examplePath -Destination $envPath
    Write-Host "Created .env from .env.example"
}

function Start-Stack() {
    Write-Step "Starting local stack"

    $dockerExe = Resolve-CommandPath "docker" "docker was not found. Install Docker Desktop first."
    $arguments = @("compose", "up", "-d")
    if (-not $NoBuild) {
        $arguments += "--build"
    }
    try {
        Invoke-NativeCommand $dockerExe $arguments
    }
    catch {
        throw "docker compose up failed. Check that Docker Desktop is running and that local ports 8080, 9090, and 3000 are free. Original error: $($_.Exception.Message)"
    }
}

function Show-FailureDiagnostics() {
    $dockerExe = Resolve-CommandPath "docker" "docker was not found. Install Docker Desktop first."
    Write-Host ""
    Write-Host "Current compose status:"
    try {
        Invoke-NativeCommand $dockerExe @("compose", "ps")
    }
    catch {
        Write-Warning $_
    }

    Write-Host ""
    Write-Host "Recent ingestion-gateway logs:"
    try {
        Invoke-NativeCommand $dockerExe @("compose", "logs", "--no-color", "--tail=120", "ingestion-gateway")
    }
    catch {
        Write-Warning $_
    }
}

function Wait-ForHealth() {
    if ($SkipHealthCheck) {
        Write-Step "Skipping health check"
        return
    }

    Write-Step "Waiting for ingestion gateway readiness"

    if ($DryRun) {
        Write-Host "[dry-run] would poll $healthUrl for up to $HealthTimeoutSeconds seconds"
        return
    }

    $deadline = (Get-Date).AddSeconds($HealthTimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $healthUrl -TimeoutSec 5
            if ($response.StatusCode -eq 200) {
                Write-Host "ingestion-gateway is healthy at $healthUrl"
                return
            }
        }
        catch {
        }
        Start-Sleep -Seconds 3
    }

    Write-Host "The stack did not become healthy within $HealthTimeoutSeconds seconds. Leaving containers running for inspection."
    Show-FailureDiagnostics
    exit 1
}

function Show-SuccessSummary() {
    Write-Host ""
    Write-Host "TeamPulse Bridge is ready."
    Write-Host "  Operator UI: http://127.0.0.1:8080/"
    Write-Host "  Health:      $healthUrl"
    Write-Host "  Metrics:     http://127.0.0.1:8080/metrics"
    Write-Host "  Prometheus:  http://127.0.0.1:9090"
    Write-Host "  Grafana:     http://127.0.0.1:3000"
    Write-Host ""
    Write-Host "Stop the stack with:"
    Write-Host "  docker compose down -v"
}

try {
    Test-DockerPrerequisites
    Initialize-LocalEnv
    Start-Stack
    Wait-ForHealth
    Show-SuccessSummary
}
catch {
    Write-Host ""
    Write-Host "Installation failed: $($_.Exception.Message)"
    exit 1
}
