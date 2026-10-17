# Read-only environment inspection and OCR of a committed synthetic PNG.
# No application helper, input API, component installation or settings writes.
$ErrorActionPreference='Stop'
if($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:RUNNER_OS -ne 'Windows') {
  throw 'This one-time diagnostic targets the hosted Windows runner'
}
Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class ReadOnlyDesktopProbe {
  [DllImport("user32.dll")] static extern IntPtr OpenInputDesktop(uint flags,bool inherit,uint access);
  [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr h);
  [DllImport("user32.dll")] static extern IntPtr GetProcessWindowStation();
  [DllImport("user32.dll")] static extern IntPtr GetThreadDesktop(uint thread);
  [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern bool GetUserObjectInformation(IntPtr h,int index,StringBuilder data,uint size,out uint needed);
  [DllImport("wtsapi32.dll")] static extern bool WTSQuerySessionInformation(IntPtr server,int session,int info,out IntPtr buffer,out int bytes);
  [DllImport("wtsapi32.dll")] static extern void WTSFreeMemory(IntPtr buffer);
  static string Name(IntPtr h){var s=new StringBuilder(256);uint n;return GetUserObjectInformation(h,2,s,512,out n)?s.ToString():"";}
  public static string Station(){return Name(GetProcessWindowStation());}
  public static string ThreadDesktop(){return Name(GetThreadDesktop(GetCurrentThreadId()));}
  public static string InputDesktop(){var h=OpenInputDesktop(0,false,1);if(h==IntPtr.Zero)return "";try{return Name(h);}finally{CloseDesktop(h);}}
  public static int SessionState(int session){IntPtr p;int n;if(!WTSQuerySessionInformation(IntPtr.Zero,session,8,out p,out n))return -1;try{return n>=4?Marshal.ReadInt32(p):-1;}finally{WTSFreeMemory(p);}}
}
'@
function Await-Result($Operation,[type]$Type) {
  $method=[System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
  } | Select-Object -First 1
  $task=$method.MakeGenericMethod($Type).Invoke($null,@($Operation))
  if(!$task.Wait(10000)){throw 'saved-image OCR operation timed out'}
  return $task.Result
}
$session=[Diagnostics.Process]::GetCurrentProcess().SessionId
$report=[ordered]@{
  diagnostic='read-only hosted Windows desktop and saved-image OCR'
  source_sha=(git rev-parse HEAD)
  run_id=$env:GITHUB_RUN_ID
  image_os=$env:ImageOS
  image_version=$env:ImageVersion
  user_interactive=[Environment]::UserInteractive
  session_id=$session
  session_state=[ReadOnlyDesktopProbe]::SessionState($session)
  window_station=[ReadOnlyDesktopProbe]::Station()
  thread_desktop=[ReadOnlyDesktopProbe]::ThreadDesktop()
  input_desktop=[ReadOnlyDesktopProbe]::InputDesktop()
  english_ocr_available=$false
  saved_image_recognized=$false
  gui_prerequisites_available=$false
  app_started=$false
  input_sent=$false
}
try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $null=[Windows.Media.Ocr.OcrEngine,Windows.Media.Ocr,ContentType=WindowsRuntime]
  $language=[Windows.Globalization.Language,Windows.Globalization,ContentType=WindowsRuntime]::new('en-US')
  $report.ocr_languages=@([Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages | ForEach-Object {$_.LanguageTag})
  $engine=[Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
  if($null -eq $engine){throw 'English OCR engine unavailable'}
  $report.english_ocr_available=$true
  $path=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../evidence/windows-flclash-app-window-2026-10-08.png'))
  $sha=[Security.Cryptography.SHA256]::Create()
  try {$hash=[BitConverter]::ToString($sha.ComputeHash([IO.File]::ReadAllBytes($path))).Replace('-','').ToLowerInvariant()} finally {$sha.Dispose()}
  $report.saved_image_sha256=$hash
  if($hash -ne '857b67b9e64987206a5bbfd537b125f3f46648d42922ac44faadfac7ba9ee546'){throw 'synthetic reference image digest mismatch'}
  $null=[Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime]
  $null=[Windows.Storage.Streams.IRandomAccessStreamWithContentType,Windows.Storage.Streams,ContentType=WindowsRuntime]
  $null=[Windows.Graphics.Imaging.BitmapDecoder,Windows.Graphics.Imaging,ContentType=WindowsRuntime]
  $null=[Windows.Graphics.Imaging.SoftwareBitmap,Windows.Graphics.Imaging,ContentType=WindowsRuntime]
  $null=[Windows.Media.Ocr.OcrResult,Windows.Media.Ocr,ContentType=WindowsRuntime]
  $file=Await-Result ([Windows.Storage.StorageFile]::GetFileFromPathAsync($path)) ([Windows.Storage.StorageFile])
  $stream=Await-Result ($file.OpenReadAsync()) ([Windows.Storage.Streams.IRandomAccessStreamWithContentType])
  try {
    $decoder=Await-Result ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $bitmap=Await-Result ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
    try {$ocr=Await-Result ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])} finally {$bitmap.Dispose()}
  } finally {$stream.Dispose()}
  $words=@($ocr.Lines | ForEach-Object {$_.Words} | ForEach-Object {@{text=$_.Text;x=$_.BoundingRect.X;y=$_.BoundingRect.Y;width=$_.BoundingRect.Width;height=$_.BoundingRect.Height}})
  $report.recognized_words=$words
  $report.saved_image_recognized=@($words | Where-Object {$_.text -ceq 'Dashboard' -and $_.x -ge 60 -and $_.y -ge 35 -and ($_.x+$_.width) -le 380 -and ($_.y+$_.height) -le 90}).Count -eq 1
} catch {$report.ocr_error=$_.Exception.Message}
$report.gui_prerequisites_available=$report.user_interactive -and $session -gt 0 -and $report.session_state -eq 0 -and $report.window_station -ceq 'WinSta0' -and $report.input_desktop -ceq 'Default' -and $report.thread_desktop -ceq 'Default' -and $report.english_ocr_available -and $report.saved_image_recognized
$report | ConvertTo-Json -Depth 6
