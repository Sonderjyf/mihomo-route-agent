$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/flclash_ui_policy.ps1"
function Record($name='Profiles', $owner=7, $enabled=$true, $offscreen=$false, $invoke=$true) {
  @{Name=$name; ProcessId=$owner; Enabled=$enabled; Offscreen=$offscreen; Invoke=$invoke}
}
$valid = Record
if ((Select-AppControl @($valid) Profiles 7) -ne $valid) { throw 'unique button rejected' }
foreach ($bad in @((Record -owner 8), (Record -enabled $false), (Record -offscreen $true),
                  (Record -invoke $false), (Record -name 'profiles'), (Record -name '配置'))) {
  if ($null -ne (Select-AppControl @($bad) Profiles 7)) { throw 'unsafe or mismatched control accepted' }
}
if ($null -ne (Select-AppControl @() Profiles 7)) { throw 'empty tree accepted' }
$rejected=$false
try { $null=Select-AppControl @($valid,$valid) Profiles 7 } catch { $rejected=$true }
if (!$rejected) { throw 'ambiguous tree accepted' }
'PASS: unique, empty, ambiguous, wrong process, hidden, disabled, wrong label and missing Invoke'

$window=@{Pid=7;Class='FLUTTER_RUNNER_WIN32_WINDOW';Visible=$true;Minimized=$false;Interactive=$true;ActiveSession=$true;Desktop='WinSta0/Default';Foreground=$true;Width=680;Height=580;Dpi=96}
Assert-PointerWindow $window 7
foreach($change in @(@{Pid=8},@{Class='other'},@{Visible=$false},@{Minimized=$true},@{Interactive=$false},@{ActiveSession=$false},@{Desktop='Service/Default'},@{Foreground=$false},@{Width=681},@{Height=579},@{Dpi=120})) {
  $candidate=$window.Clone();foreach($key in $change.Keys){$candidate[$key]=$change[$key]}
  $rejected=$false;try{Assert-PointerWindow $candidate 7}catch{$rejected=$true}
  if(!$rejected){throw 'unsafe pointer window accepted'}
}
$word=@{Text='Profiles';X=72;Y=55;Width=70;Height=25}
if($null -eq (Select-OcrWord @($word) Profiles @(60,35,380,90))){throw 'valid title rejected'}
if($null -ne (Select-OcrWord @($word) Profiles @(8,90,230,480))){throw 'title accepted as sidebar target'}
$rejected=$false;try{$null=Select-OcrWord @($word,$word) Profiles @(60,35,380,90)}catch{$rejected=$true}
if(!$rejected){throw 'ambiguous OCR target accepted'}
Add-Type -AssemblyName System.Drawing
. "$PSScriptRoot/flclash_pointer.ps1"
function Get-OwnedBitmap($Handle) {
  if($script:blank){return [Drawing.Bitmap]::new(680,580)}
  return [Drawing.Bitmap]::new((Join-Path $PSScriptRoot '../evidence/windows-flclash-app-window-2026-10-08.png'))
}
$script:blank=$false;Assert-ToggleTemplate 0
$script:blank=$true;$rejected=$false;try{Assert-ToggleTemplate 0}catch{$rejected=$true}
if(!$rejected){throw 'blank screenshot accepted as a visual anchor'}
'PASS: pointer window guards, OCR regions/ambiguity, actual saved anchor and blank-image rejection; no window/input API called'

Add-Type -Path (Join-Path $PSScriptRoot 'flclash_window.cs') # Compile only; never call native methods.
$profileImage=Join-Path $PSScriptRoot '../evidence/windows-flclash-profiles-2026-10-08.png'
$reference=[Drawing.Bitmap]::new($profileImage)
try {
  $anchor=Assert-UpdateTemplate $reference
  if($anchor.target.X -ne 592 -or $anchor.target.Y -ne 65 -or $anchor.different_pixels -ne 0){throw 'observed Update target rejected'}
  foreach($kind in @('blank','shifted','swapped','erased','dimensions')) {
    $candidate=if($kind -eq 'dimensions'){[Drawing.Bitmap]::new(679,580)}else{[Drawing.Bitmap]::new(680,580)}
    try {
      $g=[Drawing.Graphics]::FromImage($candidate)
      try {
        if($kind -eq 'shifted'){$g.DrawImageUnscaled($reference,8,0)}
        elseif($kind -in @('swapped','erased')){$g.DrawImageUnscaled($reference,0,0)}
        if($kind -eq 'erased'){
          $brush=[Drawing.SolidBrush]::new($reference.GetPixel(578,65))
          try{$g.FillRectangle($brush,580,53,24,24)}finally{$brush.Dispose()}
        }
      } finally {$g.Dispose()}
      if($kind -eq 'swapped'){
        for($x=572;$x -lt 612;$x++){for($y=45;$y -lt 85;$y++){
          $candidate.SetPixel($x,$y,$reference.GetPixel(($x+40),$y))
          $candidate.SetPixel(($x+40),$y,$reference.GetPixel($x,$y))
        }}
      }
      $rejected=$false;try{$null=Assert-UpdateTemplate $candidate}catch{$rejected=$true}
      if(!$rejected){throw "unsafe toolbar image accepted: $kind"}
    } finally {$candidate.Dispose()}
  }
} finally {$reference.Dispose()}
'PASS: pinned Update/sort toolbar identity; blank, shifted, swapped, erased and wrong-size rejection; native helper compiled without calling it'
