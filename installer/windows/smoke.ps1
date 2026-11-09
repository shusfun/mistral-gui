$ErrorActionPreference = 'Stop'
if (!$env:RUNNER_TEMP -or !$env:GITHUB_ACTIONS) { throw 'Run this installer smoke check on a disposable Actions runner' }
$smokeRoot = Join-Path $env:RUNNER_TEMP 'mistral-gui-installer-smoke'
$installRoot = Join-Path $smokeRoot 'app'
$dataRoot = Join-Path $smokeRoot 'data'
$installer = (Resolve-Path 'dist/mistral-gui-windows-x64-setup.exe').Path
$process = Start-Process -FilePath $installer -ArgumentList @('/S', "/D=$installRoot") -WindowStyle Hidden -Wait -PassThru
if ($process.ExitCode) { throw "Installer failed: $($process.ExitCode)" }
$binary = Join-Path $installRoot 'mistral-gui.exe'
if (!(Test-Path -LiteralPath $binary)) { throw 'Installed executable missing' }
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$address = '127.0.0.1:' + $listener.LocalEndpoint.Port
$listener.Stop()
$manager = Start-Process -FilePath $binary -ArgumentList @('--addr', $address, '--data-dir', "`"$dataRoot`"", '--no-open') -WindowStyle Hidden -PassThru
try {
  $healthy = $false
  for ($attempt = 0; $attempt -lt 50; $attempt++) {
    try {
      $health = Invoke-RestMethod "http://$address/healthz" -TimeoutSec 1
      if ($health.ok -and $health.service -eq 'mistral-gui') { $healthy = $true; break }
    } catch {}
    Start-Sleep -Milliseconds 200
  }
  if (!$healthy) { throw 'Installed manager failed health check' }
} finally {
  & $binary --addr $address --data-dir $dataRoot --quit
  if (!$manager.WaitForExit(10000)) { $manager.Kill(); throw 'Installed manager did not stop' }
}
$uninstaller = Join-Path $installRoot 'uninstall.exe'
$process = Start-Process -FilePath $uninstaller -ArgumentList @('/S', "_?=$installRoot") -WindowStyle Hidden -Wait -PassThru
if ($process.ExitCode) { throw "Uninstaller failed: $($process.ExitCode)" }
if (Test-Path -LiteralPath $binary) { throw 'Uninstaller left manager executable' }
if (!(Test-Path -LiteralPath (Join-Path $dataRoot 'state.json'))) { throw 'Uninstall unexpectedly deleted user data' }
Write-Output 'Silent install, installed manager health, shutdown, and uninstall passed on disposable runner.'
