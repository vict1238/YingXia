# Save immutable upstream source snapshots and original license texts.
# Files are not executed; the recipes remain source/build documentation.
$ErrorActionPreference = 'Stop'
$files = @(
  @{File='libmpv-windows-build-f5a6f879.zip';URL='https://codeload.github.com/media-kit/libmpv-win32-video-build/zip/f5a6f879c9b8bef6a73e52c8c0f35e51636c96e5'},
  @{File='libmpv-android-build-fe8c3ac1.zip';URL='https://codeload.github.com/media-kit/libmpv-android-video-build/zip/fe8c3ac1a91c09aa6fb1deccbc833f1bafa54768'},
  @{File='mpv-windows-652a1dd907.zip';URL='https://codeload.github.com/mpv-player/mpv/zip/652a1dd907'},
  @{File='mpv-android-78d43740.zip';URL='https://codeload.github.com/mpv-player/mpv/zip/78d43740f52db817d98bcf24fb30a76ab6fa13ff'},
  @{File='ffmpeg-6.0.tar.xz';URL='https://ffmpeg.org/releases/ffmpeg-6.0.tar.xz'},
  @{File='LICENSE-mpv-LGPL-2.1.txt';URL='https://raw.githubusercontent.com/mpv-player/mpv/652a1dd907/LICENSE.LGPL'},
  @{File='LICENSE-mpv-GPL-2.0.txt';URL='https://raw.githubusercontent.com/mpv-player/mpv/652a1dd907/LICENSE.GPL'},
  @{File='COPYRIGHT-mpv.txt';URL='https://raw.githubusercontent.com/mpv-player/mpv/652a1dd907/Copyright'},
  @{File='COPYING-FFmpeg-LGPL-2.1.txt';URL='https://raw.githubusercontent.com/FFmpeg/FFmpeg/n6.0/COPYING.LGPLv2.1'},
  @{File='COPYING-FFmpeg-LGPL-3.0.txt';URL='https://raw.githubusercontent.com/FFmpeg/FFmpeg/n6.0/COPYING.LGPLv3'},
  @{File='COPYING-FFmpeg-GPL-2.0.txt';URL='https://raw.githubusercontent.com/FFmpeg/FFmpeg/n6.0/COPYING.GPLv2'},
  @{File='COPYING-FFmpeg-GPL-3.0.txt';URL='https://raw.githubusercontent.com/FFmpeg/FFmpeg/n6.0/COPYING.GPLv3'},
  @{File='LICENSE-FFmpeg.txt';URL='https://raw.githubusercontent.com/FFmpeg/FFmpeg/n6.0/LICENSE.md'},
  @{File='LICENSE-ANGLE.txt';URL='https://raw.githubusercontent.com/google/angle/2693b03eba82/LICENSE'},
  @{File='AUTHORS-ANGLE.txt';URL='https://raw.githubusercontent.com/google/angle/2693b03eba82/AUTHORS'},
  @{File='LICENSE-Vulkan-loader.txt';URL='https://raw.githubusercontent.com/KhronosGroup/Vulkan-Loader/v1.3.218/LICENSE.txt'},
  @{File='LICENSE-SwiftShader.txt';URL='https://raw.githubusercontent.com/google/swiftshader/master/LICENSE.txt'},
  @{File='LICENSE-libcxx-LLVM.txt';URL='https://raw.githubusercontent.com/llvm/llvm-project/llvmorg-14.0.6/libcxx/LICENSE.TXT'},
  @{File='LICENSE-zlib.txt';URL='https://raw.githubusercontent.com/madler/zlib/v1.2.12/LICENSE'},
  @{File='Microsoft-VC-Runtime-License.docx';URL='https://visualstudio.microsoft.com/wp-content/uploads/2021/09/Visual-C-Runtime-2015-2022-License-1.docx'}
)
$manifest = @()
foreach ($file in $files) {
  $target = Join-Path $PSScriptRoot $file.File
  try {
    Invoke-WebRequest -Uri $file.URL -OutFile $target
    $manifest += [pscustomobject]@{file=$file.File;url=$file.URL;sha256=(Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant();bytes=(Get-Item -LiteralPath $target).Length;status='saved'}
  } catch {
    $manifest += [pscustomobject]@{file=$file.File;url=$file.URL;status='unavailable';error=$_.Exception.Message}
  }
}
$json = $manifest | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText((Join-Path $PSScriptRoot 'upstream-source-manifest.json'), $json, [Text.UTF8Encoding]::new($false))
$manifest | Select-Object file,status,bytes | ConvertTo-Json -Depth 3
