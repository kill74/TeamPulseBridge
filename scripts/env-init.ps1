Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$envPath = Join-Path $repoRoot ".env"
$examplePath = Join-Path $repoRoot ".env.example"

if (Test-Path $envPath) {
    Write-Host ".env already exists"
    exit 0
}

if (-not (Test-Path $examplePath)) {
    throw ".env.example was not found in the repository root."
}

Copy-Item -LiteralPath $examplePath -Destination $envPath
Write-Host "Created .env from .env.example"
