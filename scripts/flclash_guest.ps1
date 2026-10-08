param(
  [Parameter(Mandatory=$true)][ValidateSet('snapshot','clean','metadata','start','children','close','alive','terminate','profiles','update')][string]$Action,
  [string]$Executable,
  [int]$AppPid = 0,
  [string]$DiagnosticPath,
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
. "$PSScriptRoot/flclash_ui_policy.ps1"
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Collections.Generic;
public static class AppWindowEvidence {
  public delegate bool Callback(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr h, Callback cb, IntPtr p);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
  [DllImport("oleacc.dll")] static extern int AccessibleObjectFromWindow(IntPtr h, uint id, ref Guid iid, out IntPtr obj);
  public static IntPtr[] Children(IntPtr h) {
    var list=new List<IntPtr>(); EnumChildWindows(h,(w,p)=>{list.Add(w); return list.Count<128;},IntPtr.Zero); return list.ToArray();
  }
  public static int RequestAccessibility(IntPtr h) {
    // Standard MSAA request; no WM_GETOBJECT injection or system accessibility change.
    var iid=new Guid("618736e0-3c3d-11cf-810c-00aa00389b71"); IntPtr obj;
    int hr=AccessibleObjectFromWindow(h,0xfffffffc,ref iid,out obj);
    if(obj!=IntPtr.Zero) Marshal.Release(obj); return hr;
  }
}
'@
$name = if($Action -eq 'profiles') {'Profiles'} else {'Update'}
$diagnostic = [ordered]@{action=$Action;pid=$AppPid;label=$name;tree=@();accessibility_requests=@();semantics_enabled='unknown';screenshot_status='not_attempted'}
$root=$null
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
$deadline = [DateTime]::UtcNow.AddSeconds(15)
try {
do {
  $p.Refresh()
  if ($p.HasExited) { throw 'owned app exited during UI wait' }
  if ($p.MainWindowHandle -ne 0) {
    $handle=$p.MainWindowHandle
    $diagnostic.window_visible=[AppWindowEvidence]::IsWindowVisible($handle)
    $diagnostic.window_minimized=[AppWindowEvidence]::IsIconic($handle)
    if ($diagnostic.accessibility_requests.Count -eq 0) {
      foreach($hwnd in @($handle)+[AppWindowEvidence]::Children($handle)) {
        [uint32]$owner=0
        $null=[AppWindowEvidence]::GetWindowThreadProcessId($hwnd,[ref]$owner)
        if ($owner -eq $AppPid) {
          $diagnostic.accessibility_requests += @{hwnd=$hwnd.ToInt64();hresult=[AppWindowEvidence]::RequestAccessibility($hwnd)}
        }
      }
    }
    # A window handle is not a semantic-tree readiness signal. Reacquire every poll.
    $root=[Windows.Automation.AutomationElement]::FromHandle($handle)
    try { $records=@(Get-AppTree $root) } catch [Windows.Automation.ElementNotAvailableException] { continue }
    $diagnostic.tree=@($records | Select-Object Name,ProcessId,Enabled,Offscreen,Invoke,Class,Type,Patterns)
    $selected=Select-AppControl $records $name $AppPid
    if ($null -ne $selected -and $diagnostic.window_visible -and !$diagnostic.window_minimized) {
      $pattern=$selected.Element.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern)
      $pattern.Invoke()
      'true'; exit
    }
  }
  Start-Sleep -Milliseconds 200
} while ([DateTime]::UtcNow -lt $deadline)
throw "no unique visible enabled $name button before UI deadline"
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
