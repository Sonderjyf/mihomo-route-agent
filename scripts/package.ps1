param([string]$OutputRoot = 'dist')
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    $revision = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read source revision' }
    $dirty = [bool](& git status --porcelain --untracked-files=normal)
    $goVersion = & go version
    if ($LASTEXITCODE -ne 0) { throw 'Go is required in PATH' }
    $platform = (& go env GOOS GOARCH) -join '-'
    $label = 'route-agent-0.1.0-dev-' + $platform + '-' + $revision.Substring(0, 12)
    if ($dirty) { $label += '-dirty' }
    if (-not [IO.Path]::IsPathRooted($OutputRoot)) { $OutputRoot = Join-Path $repoRoot $OutputRoot }
    $destination = [IO.Path]::GetFullPath((Join-Path $OutputRoot $label))
    $archive = $destination + '.zip'
    if ((Test-Path -LiteralPath $destination) -or (Test-Path -LiteralPath $archive)) { throw 'Package output already exists' }
    New-Item -ItemType Directory -Path $destination | Out-Null
    $binaryName = 'route-agent'
    if ($platform.StartsWith('windows-')) { $binaryName += '.exe' }
    & go build -trimpath -o (Join-Path $destination $binaryName) ./cmd/route-agent
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    foreach ($name in @('LICENSE', 'THIRD_PARTY_NOTICES.md', 'README.md', 'RUNBOOK.md', 'BUILDING.md', 'PROBES.md', 'OBSERVATION.md', 'RECOVERY.md', 'ADAPTATION.md', 'config.example.json')) {
        Copy-Item -LiteralPath (Join-Path $repoRoot $name) -Destination $destination
    }
    $examples = Join-Path $destination 'examples'
    New-Item -ItemType Directory -Path $examples | Out-Null
    foreach ($name in @('observation.shadow.json', 'isolated-profile.json', 'isolated-profile.yaml')) {
        Copy-Item -LiteralPath (Join-Path (Join-Path $repoRoot 'examples') $name) -Destination $examples
    }
    $manifest = @{version='0.1.0-dev'; revision=$revision; dirty=$dirty; platform=$platform; go=$goVersion; scope='experimental; real evidence shadow only; TUN not tested'}
    $manifest | ConvertTo-Json | Set-Content -Encoding UTF8 -LiteralPath (Join-Path $destination 'manifest.json')
    $hashes = Get-ChildItem -LiteralPath $destination -File -Recurse | Sort-Object FullName | ForEach-Object {
        $relative = $_.FullName.Substring($destination.Length + 1).Replace('\', '/')
        (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $relative
    }
    $hashes | Set-Content -Encoding ASCII -LiteralPath (Join-Path $destination 'SHA256SUMS')
    Compress-Archive -LiteralPath $destination -DestinationPath $archive
    Write-Output $archive
} finally { Pop-Location }
