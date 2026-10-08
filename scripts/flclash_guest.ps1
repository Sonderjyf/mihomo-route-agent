param(
  [Parameter(Mandatory=$true)][ValidateSet('snapshot','clean','metadata','start','children','close','alive','terminate','profiles','update')][string]$Action,
  [string]$Executable,
  [int]$AppPid = 0,
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
$deadline = [DateTime]::UtcNow.AddSeconds(15)
do {
  $p.Refresh()
  if ($p.MainWindowHandle -ne 0) { break }
  Start-Sleep -Milliseconds 200
} while ([DateTime]::UtcNow -lt $deadline)
if ($p.MainWindowHandle -eq 0) { throw 'blocked_gui_automation: no app window' }
$root = [Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
$name = if($Action -eq 'profiles') {'Profiles'} else {'Update'}
$condition = [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::NameProperty,$name)
$found = $root.FindAll([Windows.Automation.TreeScope]::Descendants,$condition)
$invokable = @()
foreach($element in $found) {
  $pattern = $null
  if($element.TryGetCurrentPattern([Windows.Automation.InvokePattern]::Pattern,[ref]$pattern) -and $element.Current.IsEnabled -and !$element.Current.IsOffscreen) { $invokable += $pattern }
}
if($invokable.Count -ne 1) { throw "blocked_gui_automation: expected one invokable $name control, found $($invokable.Count)" }
$invokable[0].Invoke()
'true'
