param(
  [Parameter(Mandatory=$true)][ValidateSet('snapshot','clean','metadata','start','children','close','alive','terminate','profiles','update','inventory','preflight')][string]$Action,
  [string]$Executable,
  [int]$AppPid = 0,
  [string]$DiagnosticPath,
  [switch]$AllowWindowInput,
  [switch]$AllowIsolatedApp
)
$ErrorActionPreference = 'Stop'
if (!$AllowIsolatedApp -or $env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:RUNNER_OS -ne 'Windows' -or $env:GITHUB_REPOSITORY -ne 'Sonderjyf/mihomo-route-agent') {
  throw 'App helper is restricted to an explicitly approved disposable hosted Windows guest'
}
if ($Action -eq 'clean') {
  if (@(Get-Process -Name FlClash,FlClashCore,FlClashHelperService -ErrorAction SilentlyContinue).Count -ne 0) { throw 'Guest already has an app/core/helper process' }
  if (@(Get-NetTCPConnection -State Listen | Where-Object {$_.LocalPort -in @(9090,17891,18765,18766)}).Count -ne 0) { throw 'Guest fixture port is occupied' }
  'true'; exit
}
if ($Action -eq 'snapshot') {
  $inet = Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
  $dns = @(Get-DnsClientServerAddress | Sort-Object InterfaceIndex,AddressFamily | Select-Object InterfaceIndex,AddressFamily,ServerAddresses)
  $routes = @(Get-NetRoute | Where-Object {$_.DestinationPrefix -in @('0.0.0.0/0','::/0','198.18.0.0/15','198.19.0.0/16')} | Sort-Object DestinationPrefix,InterfaceIndex,NextHop | Select-Object DestinationPrefix,InterfaceIndex,NextHop,RouteMetric)
  [ordered]@{proxy=@{enable=$inet.ProxyEnable;server=$inet.ProxyServer;override=$inet.ProxyOverride;pac=$inet.AutoConfigURL};winhttp=@(netsh winhttp show proxy);dns=$dns;routes=$routes;adapters=@(Get-NetAdapter -IncludeHidden | Sort-Object InterfaceIndex | Select-Object InterfaceIndex,Name,Status)} | ConvertTo-Json -Depth 8 -Compress
  exit
}
if ($Action -eq 'metadata') {
  $info = [Diagnostics.FileVersionInfo]::GetVersionInfo($Executable)
  if ($info.CompanyName.Trim() -ne 'com.follow' -or $info.ProductName.Trim() -ne 'clash') { throw 'Unexpected application-support path metadata' }
  @{company=$info.CompanyName.Trim();product=$info.ProductName.Trim();roaming=[Environment]::GetFolderPath('ApplicationData')} | ConvertTo-Json -Compress
  exit
}
if ($Action -eq 'inventory') {
  $paths=@($Executable,(Join-Path (Split-Path -Parent $Executable) 'FlClashCore.exe'))
  $processes=@(Get-CimInstance Win32_Process | Where-Object {$_.ExecutablePath -in $paths} | Select-Object ProcessId,ParentProcessId,ExecutablePath)
  $ids=@($processes | ForEach-Object {$_.ProcessId})
  $tcp=@(Get-NetTCPConnection -State Listen | Where-Object {$_.OwningProcess -in $ids -or $_.LocalPort -in @(9090,17891,18765,18766)} | Select-Object LocalAddress,LocalPort,OwningProcess)
  $udp=@(Get-NetUDPEndpoint | Where-Object {$_.OwningProcess -in $ids -or $_.LocalPort -in @(9090,17891,18765,18766)} | Select-Object LocalAddress,LocalPort,OwningProcess)
  @{processes=$processes;tcp_listeners=$tcp;udp_endpoints=$udp} | ConvertTo-Json -Depth 5 -Compress
  exit
}
if ($Action -eq 'preflight') {
  Add-Type -Path (Join-Path $PSScriptRoot 'flclash_window.cs')
  . "$PSScriptRoot/flclash_pointer.ps1"
  $session=(Get-Process -Id $PID).SessionId
  $state=[ordered]@{interactive=[Environment]::UserInteractive;session=$session;active_session=[AppWindowEvidence]::ActiveSession($session);input_desktop=[AppWindowEvidence]::InputDesktop();english_ocr=$false;ready=$false}
  try {$null=New-EnglishOcrEngine;$state.english_ocr=$true} catch {$state.ocr_error=$_.Exception.Message}
  $state.ready=$state.interactive -and $state.active_session -and $state.input_desktop -ceq 'WinSta0/Default' -and $state.english_ocr
  $state | ConvertTo-Json -Compress
  exit
}
if ($Action -eq 'start') {
  $p = Start-Process -FilePath $Executable -WorkingDirectory (Split-Path -Parent $Executable) -WindowStyle Hidden -PassThru
  @{pid=$p.Id} | ConvertTo-Json -Compress
  exit
}
if ($Action -eq 'children') {
  $children = @(Get-CimInstance Win32_Process | Where-Object {$_.ParentProcessId -eq $AppPid -and $_.ExecutablePath -eq $Executable} | Select-Object ProcessId,ExecutablePath)
  ConvertTo-Json -InputObject $children -Compress
  exit
}
$p = Get-Process -Id $AppPid -ErrorAction SilentlyContinue
if (!$p) { if($Action -eq 'alive') { 'false'; exit }; throw 'Owned app process is absent' }
if ($p.Path -ne $Executable) { throw 'Recorded PID no longer belongs to the supplied executable' }
if ($Action -eq 'alive') { 'true'; exit }
if ($Action -eq 'terminate') { Stop-Process -Id $AppPid -Force; 'true'; exit }
if ($Action -eq 'close') { if(!$p.CloseMainWindow()){throw 'No application close handler'}; 'true'; exit }
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
. "$PSScriptRoot/flclash_ui_policy.ps1"
Add-Type -AssemblyName System.Drawing
Add-Type -Path (Join-Path $PSScriptRoot 'flclash_window.cs')
$name = if($Action -eq 'profiles') {'Profiles'} else {'Update'}
$diagnostic = [ordered]@{action=$Action;pid=$AppPid;label=$name;tree=@();accessibility_requests=@();semantics_enabled='unknown';screenshot_status='not_attempted'}
$root=$null
. "$PSScriptRoot/flclash_pointer.ps1"
function Get-AppTree($Root) {
  $queue=[Collections.Generic.Queue[object]]::new()
  $queue.Enqueue($Root)
  $result=@()
  $walker=[Windows.Automation.TreeWalker]::RawViewWalker
  while($queue.Count -gt 0 -and $result.Count -lt 256) {
    $element=$queue.Dequeue()
    $current=$element.Current
    $pattern=$null
    $invoke=$element.TryGetCurrentPattern([Windows.Automation.InvokePattern]::Pattern,[ref]$pattern)
    $result += [pscustomobject]@{Name=$current.Name;ProcessId=$current.ProcessId;Enabled=$current.IsEnabled;Offscreen=$current.IsOffscreen;Invoke=$invoke;Class=$current.ClassName;Type=$current.ControlType.ProgrammaticName;Patterns=@($element.GetSupportedPatterns() | ForEach-Object {$_.ProgrammaticName});Element=$element}
    $child=$walker.GetFirstChild($element)
    while($null -ne $child -and $queue.Count -lt 256) { $queue.Enqueue($child);$child=$walker.GetNextSibling($child) }
  }
  return $result
}
try {
  if (!$AllowWindowInput) {throw 'UIA-only navigation is blocked for this release; explicit --allow-window-input is required'}
  $p.Refresh()
  if ($p.MainWindowHandle -eq 0) {throw 'no owned application window'}
  $handle=$p.MainWindowHandle
  $root=[Windows.Automation.AutomationElement]::FromHandle($handle)
  $records=@(Get-AppTree $root)
  $diagnostic.tree=@($records | Select-Object Name,ProcessId,Enabled,Offscreen,Invoke,Class,Type,Patterns)
  $result=Invoke-OwnedPointerAction $handle $AppPid $Action $DiagnosticPath $diagnostic
  $result | ConvertTo-Json -Depth 6 -Compress
  exit
} catch {
  $diagnostic.error=$_.Exception.Message
  try {
    if ($null -ne $root) {
      $rect=$root.Current.BoundingRectangle
      if($rect.Width -gt 0 -and $rect.Height -gt 0 -and $rect.Width -le 1920 -and $rect.Height -le 1440) {
        $bitmap=[Drawing.Bitmap]::new([int]$rect.Width,[int]$rect.Height)
        try {
          $graphics=[Drawing.Graphics]::FromImage($bitmap)
          try { $dc=$graphics.GetHdc(); try { $ok=[AppWindowEvidence]::PrintWindow($handle,$dc,2) } finally {$graphics.ReleaseHdc($dc)} } finally {$graphics.Dispose()}
          $stream=[IO.MemoryStream]::new()
          try {
            $bitmap.Save($stream,[Drawing.Imaging.ImageFormat]::Png)
            if($ok -and $stream.Length -le 524288) {
              $diagnostic.screenshot_png_base64=[Convert]::ToBase64String($stream.ToArray())
              $diagnostic.screenshot_status='captured_owned_window_not_verified_nonblank'
            } else { $diagnostic.screenshot_status='unsupported_or_oversized' }
          } finally {$stream.Dispose()}
        } finally {$bitmap.Dispose()}
      } else {$diagnostic.screenshot_status='unsupported_dimensions'}
    }
  } catch { $diagnostic.screenshot_status='capture_failed: '+$_.Exception.Message }
  if($DiagnosticPath) { $diagnostic | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $DiagnosticPath -Encoding utf8 }
  throw "blocked_gui_automation: $($diagnostic.error)"
}
