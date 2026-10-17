param([string]$ToolsRoot=(Join-Path $PSScriptRoot '..\..\.tools\native'))
$ErrorActionPreference='Stop'
$setup=Join-Path ([IO.Path]::GetFullPath($ToolsRoot)) 'msvc\setup_x64.bat'
$compilerEnvironment=& $env:ComSpec /d /c "call `"$setup`" >nul && set"
foreach($line in $compilerEnvironment){if($line -match '^(PATH|INCLUDE|LIB)=(.*)$'){[Environment]::SetEnvironmentVariable($matches[1],$matches[2],'Process')}}
Push-Location $PSScriptRoot
try {
  New-Item -ItemType Directory -Path build -Force | Out-Null
  & rc.exe /nologo /fo build/launcher.res launcher.rc
  if($LASTEXITCODE -ne 0){throw 'Native launcher resource compilation failed.'}
  & cvtres.exe /nologo /MACHINE:X64 /OUT:resource_windows_amd64.syso build/launcher.res
  if($LASTEXITCODE -ne 0){throw 'Native launcher resource conversion failed.'}
  # Microsoft cvtres adds an absolute compiler-ID symbol that Go's COFF
  # reader cannot consume. Mark only that unused symbol as debug metadata;
  # resource sections, relocations, icon, manifest and VERSIONINFO are intact.
  $resource=Join-Path $PSScriptRoot 'resource_windows_amd64.syso'
  $bytes=[IO.File]::ReadAllBytes($resource)
  $table=[BitConverter]::ToUInt32($bytes,8)
  $count=[BitConverter]::ToUInt32($bytes,12)
  for($i=0;$i -lt $count;){
    $offset=[int]($table+18*$i)
    if($offset+18 -gt $bytes.Length){throw 'Invalid compiler resource symbol table.'}
    $name=[Text.Encoding]::ASCII.GetString($bytes,$offset,8).TrimEnd([char]0)
    if($name -eq '@comp.id' -and [BitConverter]::ToInt16($bytes,$offset+12) -eq -1){$bytes[$offset+12]=254;$bytes[$offset+13]=255}
    $i+=1+$bytes[$offset+17]
  }
  [IO.File]::WriteAllBytes($resource,$bytes)
} finally {Pop-Location}
