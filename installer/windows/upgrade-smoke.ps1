param([Parameter(Mandatory)][ValidateSet('v0.2.1','v0.3.0')][string]$PreviousVersion)
$ErrorActionPreference = 'Stop'
if (!$env:RUNNER_TEMP -or !$env:GITHUB_ACTIONS) { throw 'Run on a disposable Actions runner only' }
$runId = [guid]::NewGuid().ToString('N')
$root = Join-Path $env:RUNNER_TEMP "mistral-upgrade-$runId"
$installRoot = Join-Path $root 'app'
$dataRoot = Join-Path $root 'data'
$oldAssets = Join-Path $root 'old'
New-Item -ItemType Directory -Path $oldAssets -Force | Out-Null
gh release download $PreviousVersion --repo shusfun/mistral-gui --pattern mistral-gui-windows-x64-setup.exe --dir $oldAssets
if ($LASTEXITCODE) { throw 'Could not obtain previous installer' }
$binary = Join-Path $installRoot 'mistral-gui.exe'
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start(); $address = '127.0.0.1:' + $listener.LocalEndpoint.Port; $listener.Stop()
function Install-Package([string]$path) {
  $p = Start-Process -FilePath $path -ArgumentList @('/S', "/D=$installRoot") -WindowStyle Hidden -Wait -PassThru
  if ($p.ExitCode) { throw "Installer exited $($p.ExitCode)" }
}
function Start-Manager {
  $p = Start-Process -FilePath $binary -ArgumentList @('--addr', $address, '--data-dir', "`"$dataRoot`"", '--no-open') -WindowStyle Hidden -PassThru
  for ($i = 0; $i -lt 100; $i++) {
    try {
      $health = Invoke-RestMethod "http://$address/healthz" -TimeoutSec 1
      if ($health.ok -and $health.pid -eq $p.Id) { return $p }
      $record = Get-Content (Join-Path $dataRoot 'instance.json') -Raw | ConvertFrom-Json
      if ($p.Id -eq $record.pid -and $health.ok -and $health.instance -eq $record.instance) { return $p }
    } catch {}
    Start-Sleep -Milliseconds 100
  }
  if (!$p.HasExited) { $p.Kill() }
  throw 'Manager health failed'
}
function Stop-Manager($p) {
  & $binary --addr $address --data-dir $dataRoot --quit
  if (!$p.WaitForExit(10000)) { $p.Kill(); throw 'Manager did not stop' }
}
function Read-Token {
  $state = Get-Content (Join-Path $dataRoot 'state.json') -Raw | ConvertFrom-Json
  $token = $state.config.token
  if ($token.StartsWith('dpapi:')) {
    return [Text.Encoding]::UTF8.GetString([Security.Cryptography.ProtectedData]::Unprotect([Convert]::FromBase64String($token.Substring(6)), $null, [Security.Cryptography.DataProtectionScope]::CurrentUser))
  }
  return $token
}
Install-Package (Join-Path $oldAssets 'mistral-gui-windows-x64-setup.exe')
$old = Start-Manager
try {
  $oldHealth = Invoke-RestMethod "http://$address/healthz"
  if ($oldHealth.version -ne $PreviousVersion) { throw 'Wrong previous version' }
  $tokenBefore = Read-Token
  $headers = @{Authorization="Bearer $tokenBefore"; traceparent="00-$runId-1111111111111111-01"}
  Invoke-RestMethod "http://$address/api/v1/settings" -Method Post -Headers $headers -ContentType 'application/json' -Body '{"theme":"dark","language":"en-US"}' | Out-Null
  New-Item -ItemType Directory -Path (Join-Path $dataRoot 'models') -Force | Out-Null
  [IO.File]::WriteAllText((Join-Path $dataRoot 'models/retained.fixture'), 'retained model data')
} finally { Stop-Manager $old }
Install-Package (Resolve-Path 'dist/mistral-gui-windows-x64-setup.exe').Path
$next = Start-Manager
try {
  $health = Invoke-RestMethod "http://$address/healthz"
  if ($health.version -ne $env:VERSION) { throw 'New version not running' }
  if ((Read-Token) -ne $tokenBefore) { throw 'Credential changed during upgrade' }
  $status = Invoke-RestMethod "http://$address/api/v1/status" -Headers $headers
  if ($status.settings.theme -ne 'dark' -or $status.settings.language -ne 'en-US') { throw 'Settings lost' }
  if ([IO.File]::ReadAllText((Join-Path $dataRoot 'models/retained.fixture')) -ne 'retained model data') { throw 'Model data lost' }
  $info = (Get-Item $binary).VersionInfo
  if ($info.ProductName -ne 'mistral' -or $info.FileDescription -ne 'mistral') { throw 'Brand resource missing' }
  $reg = Get-ItemProperty 'HKCU:/Software/Microsoft/Windows/CurrentVersion/Uninstall/mistral-gui'
  if ($reg.DisplayName -ne 'mistral' -or !$reg.DisplayIcon) { throw 'System app branding missing' }
  $expectedVersion = $env:VERSION.TrimStart('v')
  if ($info.ProductVersion -ne $expectedVersion -or $reg.DisplayVersion -ne $expectedVersion) { throw 'Installed version metadata mismatch' }
  $desktop = [Environment]::GetFolderPath('Desktop')
  if (!(Test-Path (Join-Path $desktop 'mistral.lnk')) -or (Test-Path (Join-Path $desktop 'mistral-gui.lnk'))) { throw 'Shortcut migration failed' }
} finally { Stop-Manager $next }
$report = @{runId=$runId; from=$oldHealth.version; to=$health.version; result='passed'; checks=@('cover-install','credentials','settings','model-data','shortcut-migration','version-resource')}
$report | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $root 'result.json')
Write-Output "SCENARIO_END $runId passed $root"
