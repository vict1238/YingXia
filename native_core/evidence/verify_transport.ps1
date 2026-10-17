param([switch]$Fixture, [string]$BackendName)
$ErrorActionPreference = 'Stop'
$taskCoreRoot = (Get-Item (Join-Path $PSScriptRoot '..')).FullName
$taskDataDir = Join-Path (Join-Path $taskCoreRoot 'build') ('native transport 中文 ' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskDataDir -Force | Out-Null
$seed = @{schema=1;settings=@{network_consent=$false};favorites=@(@{id='saved';title='收藏保留'});history=@(@{id='recent';film_id='saved';title='测试记录';episode='第 1 集';source='fixture';evidence='opened';created_at='2026-10-14T00:00:00Z'});bindings=@()}
[IO.File]::WriteAllText((Join-Path $taskDataDir 'library.json'), ($seed | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
$backendName = if ($BackendName) { $BackendName } elseif ($Fixture) { 'yingxia_backend_fixture.exe' } else { 'yingxia_backend.exe' }
$si = [Diagnostics.ProcessStartInfo]::new()
$si.FileName = Join-Path (Join-Path $taskCoreRoot 'build') $backendName
$si.UseShellExecute = $false
$si.CreateNoWindow = $true
$si.RedirectStandardInput = $true
$si.RedirectStandardOutput = $true
$si.RedirectStandardError = $true
$si.ArgumentList.Add('--data-dir')
$si.ArgumentList.Add($taskDataDir)
$proc = [Diagnostics.Process]::Start($si)
$checks = [Collections.Generic.List[string]]::new()
try {
    $lineTask = $proc.StandardOutput.ReadLineAsync()
    if (-not $lineTask.Wait(20000)) { throw 'Bootstrap timeout' }
    $boot = $lineTask.Result | ConvertFrom-Json
    if ($boot.error) { throw $boot.error }
    $checks.Add('GUI child stdout bootstrap')
    $headers = @{Cookie=$boot.cookie;Origin=$boot.origin;'X-YingXia-CSRF'=$boot.csrf}
    $state = Invoke-RestMethod -Uri ($boot.origin+'/api/state') -Headers $headers
    if ($state.data.version -ne '2.0.0-native') { throw 'Version mismatch' }
    $checks.Add('authenticated private state API')
    $before = Invoke-RestMethod -Uri ($boot.origin+'/api/library') -Headers $headers
    if ($before.data.history.Count -ne 1) { throw 'Seed missing' }
    $clear = Invoke-RestMethod -Uri ($boot.origin+'/api/clear-history') -Method Post -ContentType 'application/json' -Headers $headers -Body '{}'
    if (-not $clear.ok) { throw 'Clear failed' }
    $checks.Add('history clears through actual process API')
    $after = Invoke-RestMethod -Uri ($boot.origin+'/api/library') -Headers $headers
    if ($after.data.history.Count -ne 0 -or $after.data.favorites.Count -ne 1) { throw 'Library incorrect' }
    $checks.Add('favorites preserved')
    $disk = Get-Content -LiteralPath (Join-Path $taskDataDir 'library.json') -Raw | ConvertFrom-Json
    if ($disk.history.Count -ne 0 -or $disk.favorites.Count -ne 1) { throw 'Disk incorrect' }
    $checks.Add('library disk write verified')
    if ($Fixture) {
        $job = (Invoke-RestMethod -Uri ($boot.origin+'/api/search/start') -Method Post -ContentType 'application/json' -Headers $headers -Body '{"query":"原生播放测试","mode":"all"}').data
        for ($i=0; $i -lt 30 -and $job.phase -ne 'done'; $i++) {
            Start-Sleep -Milliseconds 100
            $job = (Invoke-RestMethod -Uri ($boot.origin+'/api/search/status?id='+$job.id) -Headers $headers).data
        }
        if ($job.phase -ne 'done' -or $job.items.Count -ne 2 -or $job.available_count -ne 2) { throw 'Fixture availability search failed' }
        $checks.Add('fixture async search verifies two media candidates')
        foreach ($item in $job.items) {
            $detail = (Invoke-RestMethod -Uri ($boot.origin+'/api/detail?id='+[uri]::EscapeDataString($item.id)) -Headers $headers).data
            $group = $detail.groups[0]
            $episode = $group.episodes[0]
            $body = @{film_id=$detail.id;group_key=$group.key;episode_key=$episode.key;source='native'} | ConvertTo-Json
            $play = (Invoke-RestMethod -Uri ($boot.origin+'/api/resolve') -Method Post -ContentType 'application/json' -Headers $headers -Body $body).data
            if ($play.kind -ne 'media' -or $play.url -notlike '/api/media?ticket=*') { throw 'Fixture resolve did not use private relay' }
            $checks.Add('fixture resolve media ticket '+$item.title)
            $media = Invoke-WebRequest -Uri ($boot.origin+$play.url) -Headers $headers
            if ($media.StatusCode -ne 200) { throw 'Fixture media request failed' }
            if ($play.format -eq 'hls') {
                $playlist = [Text.Encoding]::UTF8.GetString($media.Content)
                $child = @($playlist -split "`n" | Where-Object { $_ -and -not $_.StartsWith('#') })[0].Trim()
                $variant = Invoke-WebRequest -Uri ($boot.origin+$child) -Headers $headers
                $variantText = [Text.Encoding]::UTF8.GetString($variant.Content)
                $segment = @($variantText -split "`n" | Where-Object { $_ -and -not $_.StartsWith('#') })[0].Trim()
                $segmentRes = Invoke-WebRequest -Uri ($boot.origin+$segment) -Headers $headers
                if ($segmentRes.StatusCode -ne 200 -or $segmentRes.RawContentLength -le 188) { throw 'HLS segment relay failed' }
                $checks.Add('fixture HLS master variant segment authenticated relay')
            } else {
                $rangeHeaders = $headers.Clone()
                $rangeHeaders.Range = 'bytes=0-63'
                $range = Invoke-WebRequest -Uri ($boot.origin+$play.url) -Headers $rangeHeaders
                if ($range.StatusCode -ne 206 -or $range.RawContentLength -ne 64) { throw 'WebM range relay failed' }
                $checks.Add('fixture WebM byte range authenticated relay')
            }
        }
        [void](Invoke-RestMethod -Uri ($boot.origin+'/api/clear-history') -Method Post -ContentType 'application/json' -Headers $headers -Body '{}')
    }
    [void](Invoke-RestMethod -Uri ($boot.origin+'/api/quit') -Method Post -ContentType 'application/json' -Headers $headers -Body '{}')
    if (-not $proc.WaitForExit(8000)) { throw 'API quit did not exit' }
    $checks.Add('authenticated quit exits child')
    $proc2 = [Diagnostics.Process]::Start($si)
    try {
        $lineTask2 = $proc2.StandardOutput.ReadLineAsync()
        if (-not $lineTask2.Wait(20000)) { throw 'Restart timeout' }
        $boot2 = $lineTask2.Result | ConvertFrom-Json
        if ($boot2.error) { throw $boot2.error }
        $checks.Add('restart releases data lock')
        $headers2 = @{Cookie=$boot2.cookie;Origin=$boot2.origin;'X-YingXia-CSRF'=$boot2.csrf}
        $again = (Invoke-RestMethod -Uri ($boot2.origin+'/api/library') -Headers $headers2).data
        if ($again.history.Count -ne 0 -or $again.favorites.Count -ne 1) { throw 'History clear lost across process restart' }
        $checks.Add('cleared history persists across process restart')
        $proc2.StandardInput.Close()
        if (-not $proc2.WaitForExit(8000)) { throw 'Pipe close did not exit' }
        $checks.Add('host stdin close exits child')
    } finally {
        if (-not $proc2.HasExited) { $proc2.Kill() }
        $proc2.Dispose()
    }
    $report = @{version=$state.data.version;fixture=[bool]$Fixture;tested_at=(Get-Date).ToUniversalTime().ToString('o');binary_sha256=(Get-FileHash -LiteralPath $si.FileName -Algorithm SHA256).Hash.ToLower();checks=$checks.ToArray();passed=$checks.Count}
    $outName = if ($Fixture) { 'windows_fixture_transport.json' } else { 'windows_transport.json' }
    [IO.File]::WriteAllText((Join-Path $PSScriptRoot $outName), ($report | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
    $report | ConvertTo-Json -Depth 8
} finally {
    if (-not $proc.HasExited) { $proc.Kill() }
    $proc.Dispose()
}
