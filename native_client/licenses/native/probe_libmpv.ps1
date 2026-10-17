# Read native player build metadata without opening any media or network URL.
param([string]$LibraryPath = (Join-Path $PSScriptRoot '..\..\build\windows\native_cmake\libmpv\libmpv-2.dll'))
$ErrorActionPreference = 'Stop'
$LibraryPath = [IO.Path]::GetFullPath($LibraryPath)
$escaped = $LibraryPath.Replace('"', '""')
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public static class YingXiaMpvBuildMetadata {
 [DllImport(@"$escaped", CallingConvention=CallingConvention.Cdecl)] public static extern IntPtr mpv_create();
 [DllImport(@"$escaped", CallingConvention=CallingConvention.Cdecl)] public static extern int mpv_initialize(IntPtr p);
 [DllImport(@"$escaped", CallingConvention=CallingConvention.Cdecl)] public static extern IntPtr mpv_get_property_string(IntPtr p, string key);
 [DllImport(@"$escaped", CallingConvention=CallingConvention.Cdecl)] public static extern void mpv_free(IntPtr p);
 [DllImport(@"$escaped", CallingConvention=CallingConvention.Cdecl)] public static extern void mpv_terminate_destroy(IntPtr p);
}
"@
$handle = [YingXiaMpvBuildMetadata]::mpv_create()
if ($handle -eq [IntPtr]::Zero) { throw 'mpv_create failed' }
try {
  $code = [YingXiaMpvBuildMetadata]::mpv_initialize($handle)
  if ($code -lt 0) { throw "mpv_initialize failed $code" }
  $data = [ordered]@{file='libmpv-2.dll'; sha256=(Get-FileHash -LiteralPath $LibraryPath -Algorithm SHA256).Hash.ToLowerInvariant()}
  foreach ($key in @('mpv-version','ffmpeg-version','mpv-configuration','mpv-license')) {
    $ptr = [YingXiaMpvBuildMetadata]::mpv_get_property_string($handle, $key)
    if ($ptr -ne [IntPtr]::Zero) {
      try { $data[$key] = [Runtime.InteropServices.Marshal]::PtrToStringUTF8($ptr) }
      finally { [YingXiaMpvBuildMetadata]::mpv_free($ptr) }
    } else { $data[$key] = $null }
  }
  $json = $data | ConvertTo-Json -Depth 4
  [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'libmpv-windows-binary.json'), $json, [Text.UTF8Encoding]::new($false))
  $json
} finally { [YingXiaMpvBuildMetadata]::mpv_terminate_destroy($handle) }
