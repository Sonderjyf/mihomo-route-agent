param([string]$OutputRoot = 'dist')
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    $revision = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read source revision' }
    $dirty = [bool](& git status --porcelain --untracked-files=normal)
    if ($dirty) { throw 'Commit reviewed source and documentation before packaging' }
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
    $documents = @(& git ls-files -- '*.md') | Where-Object { -not $_.Contains('/') }
    foreach ($name in @('LICENSE') + $documents) {
        Copy-Item -LiteralPath (Join-Path $repoRoot $name) -Destination $destination
    }
    Copy-Item -LiteralPath (Join-Path $repoRoot 'examples/portable.off.json') -Destination (Join-Path $destination 'config.example.json')
    $examples = Join-Path $destination 'examples'
    New-Item -ItemType Directory -Path $examples | Out-Null
    foreach ($name in @('observation.shadow.json', 'isolated-profile.json', 'isolated-profile.yaml', 'maintenance.template.json', 'portable.off.json')) {
        Copy-Item -LiteralPath (Join-Path (Join-Path $repoRoot 'examples') $name) -Destination $examples
    }
    $evidence = Join-Path $destination 'evidence'
    New-Item -ItemType Directory -Path $evidence | Out-Null
    Copy-Item -LiteralPath (Join-Path $repoRoot 'evidence/windows-lifecycle-success-2026-10-08.json') -Destination $evidence
    $manifest = @{version='0.1.0-dev'; revision=$revision; dirty=$dirty; platform=$platform; go=$goVersion; entry_guide='PORTABLE_QUICKSTART.md'; scope='isolated synthetic Windows lifecycle accepted; real egress/model and FlClash nonempty integration unaccepted'; lifecycle_tested_sha='39bea08f169d4098fd8884a097b628b03f35f9aa'; lifecycle_run=37753376144; current_tun_tested=$false; default_controller=''; default_mode='off'}
    $manifest | ConvertTo-Json | Set-Content -Encoding UTF8 -LiteralPath (Join-Path $destination 'manifest.json')
    $hashes = Get-ChildItem -LiteralPath $destination -File -Recurse | Sort-Object FullName | ForEach-Object {
        $relative = $_.FullName.Substring($destination.Length + 1).Replace('\', '/')
        (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $relative
    }
    $hashes | Set-Content -Encoding ASCII -LiteralPath (Join-Path $destination 'SHA256SUMS')
    Compress-Archive -LiteralPath $destination -DestinationPath $archive
    ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + [IO.Path]::GetFileName($archive)) | Set-Content -Encoding ASCII -LiteralPath ($archive + '.sha256')
    Write-Output $archive
} finally { Pop-Location }
