# Pure selector policy, also exercised with synthetic records off the desktop.
function Select-AppControl($Records, [string]$Name, [int]$OwnerPid) {
  $matches = @($Records | Where-Object {
    $_.Name -ceq $Name -and $_.ProcessId -eq $OwnerPid -and
    $_.Enabled -and !$_.Offscreen -and $_.Invoke
  })
  if ($matches.Count -gt 1) { throw "ambiguous enabled $Name controls" }
  if ($matches.Count -eq 1) { return $matches[0] }
  return $null
}

function Assert-PointerWindow($State, [int]$OwnerPid) {
  if ($State.Pid -ne $OwnerPid -or $State.Class -cne 'FLUTTER_RUNNER_WIN32_WINDOW' -or
      !$State.Visible -or $State.Minimized -or !$State.Interactive -or !$State.ActiveSession -or
      $State.Desktop -cne 'WinSta0/Default' -or !$State.Foreground -or
      $State.Width -ne 680 -or $State.Height -ne 580 -or $State.Dpi -ne 96) {
    throw 'blocked_window_input: require owned foreground 680x580 window at 96 DPI on active interactive WinSta0/Default'
  }
}

function Select-OcrWord($Words, [string]$Text, $Box) {
  $found=@($Words | Where-Object {
    $_.Text -ceq $Text -and $_.Width -gt 0 -and $_.Height -gt 0 -and
    $_.X -ge $Box[0] -and $_.Y -ge $Box[1] -and
    ($_.X+$_.Width) -le $Box[2] -and ($_.Y+$_.Height) -le $Box[3]
  })
  if($found.Count -gt 1){throw "ambiguous OCR target $Text"}
  if($found.Count -eq 1){return $found[0]}
  return $null
}
