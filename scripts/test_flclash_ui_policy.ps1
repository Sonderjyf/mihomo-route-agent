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
