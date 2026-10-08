param([Parameter(Mandatory=$true)][string]$Archive)
$ErrorActionPreference = 'Stop'
$archivePath = (Resolve-Path -LiteralPath $Archive).Path
$sandbox = Join-Path ([IO.Path]::GetTempPath()) ('route-agent-package-smoke-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $sandbox | Out-Null
try {
    $zipHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $expectedZip = (Get-Content -LiteralPath ($archivePath + '.sha256') -Raw).Split(' ')[0]
    if ($zipHash -ne $expectedZip) { throw 'ZIP checksum mismatch' }
    Expand-Archive -LiteralPath $archivePath -DestinationPath $sandbox
    $roots = @(Get-ChildItem -LiteralPath $sandbox -Directory)
    if ($roots.Count -ne 1) { throw 'Expected one package root' }
    $root = $roots[0].FullName
    $manifest = Get-Content -LiteralPath (Join-Path $root 'manifest.json') -Raw | ConvertFrom-Json
    if ($manifest.dirty -or $manifest.platform -ne 'windows-amd64' -or $manifest.revision -notmatch '^[a-f0-9]{40}$') { throw 'Invalid source/platform manifest' }
    $entries = @(Get-Content -LiteralPath (Join-Path $root 'SHA256SUMS'))
    foreach ($entry in $entries) {
        if ($entry -notmatch '^([a-f0-9]{64})  (.+)$') { throw 'Malformed checksum entry' }
        $expected, $relative = $Matches[1], $Matches[2]
        $target = [IO.Path]::GetFullPath((Join-Path $root $relative))
        if (-not $target.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Checksum path escaped package' }
        if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw 'File checksum mismatch' }
    }
    $files = @(Get-ChildItem -LiteralPath $root -Recurse -File)
    if ($files.Count -ne $entries.Count + 1) { throw 'Unmanifested package file' }
    if (@($files | Where-Object { $_.Extension -in @('.key','.env','.pem','.log') -or $_.Name -match 'ownership|session-state|\.test\.exe$' }).Count) { throw 'Private/test-only artifact in package' }
    $configPath = Join-Path $root 'config.example.json'
    $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
    if ($config.mode -ne 'off' -or $config.controller -ne '' -or $config.api_proxy -ne '') { throw 'Unsafe default config' }
    $exe = Join-Path $root 'route-agent.exe'
    $version = & $exe version
    if ($LASTEXITCODE -ne 0 -or $version -ne '0.1.0-dev') { throw 'Packaged executable version failed' }
    $null = & $exe check --config $configPath
    if ($LASTEXITCODE -ne 0) { throw 'Default offline check failed' }
    $null = & $exe explain --config $configPath example.com
    if ($LASTEXITCODE -ne 0) { throw 'Offline explain failed' }
    $templatePath = Join-Path $root 'examples/maintenance.template.json'
    $ErrorActionPreference = 'Continue'
    $null = & $exe check --config $templatePath --allow-lab-fixtures 2>&1
    $templateExit = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($templateExit -eq 0) { throw 'Unselected maintenance template was accepted' }
    $template = Get-Content -LiteralPath $templatePath -Raw | ConvertFrom-Json
    if ($template.controller -ne '' -or $template.judge -ne 'stub' -or (@($template.observation.hosts) -join ',') -ne 'example.com,www.cloudflare.com') { throw 'Unexpected template target or controller' }
    $template.maintenance.start='03:17'; $template.maintenance.timezone='UTC'
    $template.maintenance.duration_seconds=300; $template.maintenance.resume='manual'
    $selected = Join-Path $sandbox 'selected-example.json'
    [IO.File]::WriteAllText($selected, ($template | ConvertTo-Json -Depth 8), (New-Object Text.UTF8Encoding($false)))
    $null = & $exe check --config $selected --allow-lab-fixtures
    if ($LASTEXITCODE -ne 0) { throw 'Operator-selected maintenance configuration rejected' }
    $evidence = Get-Content -LiteralPath (Join-Path $root 'evidence/windows-lifecycle-success-2026-10-08.json') -Raw | ConvertFrom-Json
    if (-not $evidence.raw_result.passed -or $evidence.verification.tested_sha -ne $manifest.lifecycle_tested_sha) { throw 'Acceptance evidence mismatch' }
    [ordered]@{passed=$true; source_revision=$manifest.revision; zip_sha256=$zipHash; files_verified=$entries.Count; defaults='off, empty controller and API proxy'; maintenance='placeholder rejected, operator-selected example accepted'; network_requests=0; core_app_tun_started=$false} | ConvertTo-Json
} finally {
    $resolved = [IO.Path]::GetFullPath($sandbox)
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolved.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notmatch '^route-agent-package-smoke-[a-f0-9]{32}$') { throw 'Refusing cleanup outside owned temporary directory' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
