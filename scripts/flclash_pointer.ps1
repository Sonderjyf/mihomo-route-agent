# Loaded only by the guarded guest helper; defining these functions performs no input.
function Get-OwnedWindowState($Handle, [int]$OwnerPid) {
  [uint32]$actual=0
  $null=[AppWindowEvidence]::GetWindowThreadProcessId($Handle,[ref]$actual)
  $rect=[AppWindowEvidence+Rect]::new()
  if(![AppWindowEvidence]::GetWindowRect($Handle,[ref]$rect)){throw 'owned window rectangle unavailable'}
  $class=[Text.StringBuilder]::new(128)
  $null=[AppWindowEvidence]::GetClassName($Handle,$class,128)
  $session=(Get-Process -Id $OwnerPid).SessionId
  return @{Pid=$actual;Class=$class.ToString();Left=$rect.Left;Top=$rect.Top;Width=($rect.Right-$rect.Left);Height=($rect.Bottom-$rect.Top);Dpi=[AppWindowEvidence]::GetDpiForWindow($Handle);Visible=[AppWindowEvidence]::IsWindowVisible($Handle);Minimized=[AppWindowEvidence]::IsIconic($Handle);Foreground=([AppWindowEvidence]::GetForegroundWindow() -eq $Handle);Interactive=[Environment]::UserInteractive;ActiveSession=([AppWindowEvidence]::ActiveSession($session) -and $session -eq (Get-Process -Id $PID).SessionId);Desktop=[AppWindowEvidence]::InputDesktop()}
}

function Get-OwnedBitmap($Handle) {
  $bitmap=[Drawing.Bitmap]::new(680,580)
  $graphics=[Drawing.Graphics]::FromImage($bitmap)
  try {
    $dc=$graphics.GetHdc()
    try {$ok=[AppWindowEvidence]::PrintWindow($Handle,$dc,2)} finally {$graphics.ReleaseHdc($dc)}
  } finally {$graphics.Dispose()}
  if(!$ok){$bitmap.Dispose();throw 'owned PrintWindow failed'}
  return $bitmap
}

function Wait-WinRT($Operation, [type]$ResultType) {
  $method=[System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
  } | Select-Object -First 1
  $task=$method.MakeGenericMethod($ResultType).Invoke($null,@($Operation))
  if(!$task.Wait(5000)){throw 'local OCR operation timed out'}
  return $task.Result
}

function Read-OwnedOcr($Handle, $Path, $Engine) {
  $bitmap=Get-OwnedBitmap $Handle
  try {$bitmap.Save($Path,[Drawing.Imaging.ImageFormat]::Png)} finally {$bitmap.Dispose()}
  $file=Wait-WinRT ([Windows.Storage.StorageFile]::GetFileFromPathAsync($Path)) ([Windows.Storage.StorageFile])
  $stream=Wait-WinRT ($file.OpenReadAsync()) ([Windows.Storage.Streams.IRandomAccessStreamWithContentType])
  try {
    $decoder=Wait-WinRT ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $software=Wait-WinRT ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
    try {$result=Wait-WinRT ($Engine.RecognizeAsync($software)) ([Windows.Media.Ocr.OcrResult])} finally {$software.Dispose()}
  } finally {$stream.Dispose()}
  $words=@()
  foreach($line in $result.Lines){foreach($word in $line.Words){
    $r=$word.BoundingRect
    $words+=@{Text=$word.Text;X=$r.X;Y=$r.Y;Width=$r.Width;Height=$r.Height}
    if($words.Count -gt 300){throw 'unexpected OCR volume'}
  }}
  return $words
}

function Assert-ToggleTemplate($Handle) {
  $path=Join-Path $PSScriptRoot '../evidence/windows-flclash-app-window-2026-10-08.png'
  $sha=[Security.Cryptography.SHA256]::Create()
  try {$hash=[BitConverter]::ToString($sha.ComputeHash([IO.File]::ReadAllBytes($path))).Replace('-','')} finally {$sha.Dispose()}
  if($hash -ne '857b67b9e64987206a5bbfd537b125f3f46648d42922ac44faadfac7ba9ee546'){throw 'reference screenshot changed'}
  $reference=[Drawing.Bitmap]::new($path)
  $actual=Get-OwnedBitmap $Handle
  try {
    $bad=0
    for($x=18;$x -lt 46;$x++){for($y=45;$y -lt 75;$y++){
      $a=$actual.GetPixel($x,$y);$b=$reference.GetPixel($x,$y)
      if([Math]::Abs([int]$a.R-$b.R) -gt 10 -or [Math]::Abs([int]$a.G-$b.G) -gt 10 -or [Math]::Abs([int]$a.B-$b.B) -gt 10){$bad++}
    }}
    if($bad -gt 8){throw 'sidebar toggle visual anchor differs from observed release'}
  } finally {$reference.Dispose();$actual.Dispose()}
}

function Send-OwnedPointer($Handle, [int]$OwnerPid, $Origin, [int]$X, [int]$Y, [bool]$Click) {
  $state=Get-OwnedWindowState $Handle $OwnerPid
  Assert-PointerWindow $state $OwnerPid
  if($state.Left -ne $Origin.Left -or $state.Top -ne $Origin.Top){throw 'window moved during targeting'}
  if($X -lt 8 -or $X -ge 672 -or $Y -lt 33 -or $Y -ge 572){throw 'target outside owned app content'}
  $point=[AppWindowEvidence+Point]::new(($state.Left+$X),($state.Top+$Y))
  $child=[AppWindowEvidence]::WindowFromPoint($point)
  [uint32]$childOwner=0
  $null=[AppWindowEvidence]::GetWindowThreadProcessId($child,[ref]$childOwner)
  $class=[Text.StringBuilder]::new(128);$null=[AppWindowEvidence]::GetClassName($child,$class,128)
  if($childOwner -ne $OwnerPid -or $class.ToString() -cne 'FLUTTERVIEW'){throw 'target is occluded or not owned Flutter view'}
  [AppWindowEvidence]::Pointer($Handle,$child,$point.X,$point.Y,$Click)
}

function Invoke-OwnedPointerAction($Handle, [int]$OwnerPid, $Action, $DiagnosticPath, $Diagnostic) {
  if(!$DiagnosticPath){throw 'window input requires a diagnostic output path'}
  $before=Get-OwnedWindowState $Handle $OwnerPid
  $Diagnostic.pointer_window=$before
  $precheck=$before.Clone();$precheck.Foreground=$true
  Assert-PointerWindow $precheck $OwnerPid
  $engine=New-EnglishOcrEngine
  $null=[AppWindowEvidence]::SetForegroundWindow($Handle)
  Start-Sleep -Milliseconds 200
  $origin=Get-OwnedWindowState $Handle $OwnerPid
  $Diagnostic.pointer_window=$origin
  Assert-PointerWindow $origin $OwnerPid
  $imagePath=[IO.Path]::ChangeExtension($DiagnosticPath,'.ocr.png')
  $words=@(Read-OwnedOcr $Handle $imagePath $engine);$Diagnostic.last_ocr=$words
  $title=Select-OcrWord $words 'Profiles' @(60,35,380,90)
  if($Action -eq 'profiles') {
    if($null -eq $title) {
      if($null -eq (Select-OcrWord $words 'Dashboard' @(60,35,380,90))){throw 'unexpected page before navigation'}
      Assert-ToggleTemplate $Handle
      Send-OwnedPointer $Handle $OwnerPid $origin 32 59 $true
      Start-Sleep -Milliseconds 600
      $words=@(Read-OwnedOcr $Handle $imagePath $engine);$Diagnostic.last_ocr=$words
      $target=Select-OcrWord $words 'Profiles' @(8,90,230,480)
      if($null -eq $target){throw 'expanded sidebar Profiles label not uniquely recognized'}
      Send-OwnedPointer $Handle $OwnerPid $origin ([int]($target.X+$target.Width/2)) ([int]($target.Y+$target.Height/2)) $true
      $deadline=[DateTime]::UtcNow.AddSeconds(5)
      do {
        Start-Sleep -Milliseconds 300
        $words=@(Read-OwnedOcr $Handle $imagePath $engine);$Diagnostic.last_ocr=$words
        $title=Select-OcrWord $words 'Profiles' @(60,35,380,90)
      } while($null -eq $title -and [DateTime]::UtcNow -lt $deadline)
      if($null -eq $title){throw 'Profiles page title not observed after navigation'}
    }
    return @{method='owned-window-pointer-ocr';page_title_verified='Profiles';action='profiles'}
  }
  if($null -eq $title){throw 'Update requires verified Profiles page'}
  # At most three source-defined toolbar slots; only hover until Update is read.
  foreach($x in @(632,584,536)) {
    Send-OwnedPointer $Handle $OwnerPid $origin $x 65 $false
    Start-Sleep -Milliseconds 1000
    $words=@(Read-OwnedOcr $Handle $imagePath $engine);$Diagnostic.last_ocr=$words
    if($null -eq (Select-OcrWord $words 'Profiles' @(60,35,380,90))){throw 'page changed while locating Update'}
    if($null -ne (Select-OcrWord $words 'Update' @(($x-80),85,([Math]::Min(672,$x+80)),160))) {
      Send-OwnedPointer $Handle $OwnerPid $origin $x 65 $true
      return @{method='owned-window-pointer-ocr';page_title_verified='Profiles';tooltip_verified='Update';action='update';result_validation_required=$true}
    }
  }
  throw 'Update tooltip not verified; no toolbar click performed'
}

function New-EnglishOcrEngine {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $null=[Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime]
  $null=[Windows.Storage.Streams.IRandomAccessStreamWithContentType,Windows.Storage.Streams,ContentType=WindowsRuntime]
  $null=[Windows.Graphics.Imaging.BitmapDecoder,Windows.Graphics.Imaging,ContentType=WindowsRuntime]
  $null=[Windows.Graphics.Imaging.SoftwareBitmap,Windows.Graphics.Imaging,ContentType=WindowsRuntime]
  $null=[Windows.Media.Ocr.OcrResult,Windows.Media.Ocr,ContentType=WindowsRuntime]
  $null=[Windows.Media.Ocr.OcrEngine,Windows.Media.Ocr,ContentType=WindowsRuntime]
  $language=[Windows.Globalization.Language,Windows.Globalization,ContentType=WindowsRuntime]::new('en-US')
  $engine=[Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
  if($null -eq $engine){throw 'English OCR unavailable; no input attempted'}
  return $engine
}
