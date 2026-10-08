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
