param(
    [ValidateSet('Snapshot','Minimize','Restore','Close','Processes','CaptureFrame')][string]$Action = 'Snapshot',
    [int]$OwnerPid,
    [string]$ClassName,
    [string]$OutputPath
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
if ($Action -eq 'Processes') {
    $all = @(Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name,CommandLine)
    $ids = [System.Collections.Generic.HashSet[int]]::new()
    [void]$ids.Add($OwnerPid)
    do {
        $changed = $false
        foreach ($item in $all) {
            if ($ids.Contains([int]$item.ParentProcessId) -and $ids.Add([int]$item.ProcessId)) { $changed = $true }
        }
    } while ($changed)
    $descendants = @($all | Where-Object { $ids.Contains([int]$_.ProcessId) })
    $browsers = @($all | Where-Object { $_.Name -in @('chrome.exe','msedge.exe') } )
    @{descendants=$descendants; browsers=$browsers} | ConvertTo-Json -Depth 8 -Compress
    exit
}
Add-Type -ReferencedAssemblies System.Drawing -TypeDefinition @'
using System;
using System.Text;
using System.Collections.Generic;
using System.Runtime.InteropServices;
public static class YingXiaWindowTest {
  [StructLayout(LayoutKind.Sequential)] public struct Rect { public int left, top, right, bottom; }
  [StructLayout(LayoutKind.Sequential)] public struct MonitorInfo { public int size; public Rect monitor, work; public uint flags; }
  public delegate bool EnumProc(IntPtr hwnd, IntPtr parameter);
  [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc callback, IntPtr param);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint process);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd, StringBuilder text, int count);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int count);
  [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr hwnd, out Rect rect);
  [DllImport("user32.dll")] static extern bool GetClientRect(IntPtr hwnd, out Rect rect);
  [DllImport("user32.dll")] static extern bool IsIconic(IntPtr hwnd);
  [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hwnd);
  [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr hwnd);
  [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr hwnd, int command);
  [DllImport("user32.dll")] static extern bool PostMessage(IntPtr hwnd, uint message, IntPtr wparam, IntPtr lparam);
  [DllImport("user32.dll", EntryPoint="GetWindowLongPtrW")] static extern IntPtr GetWindowLongPtr(IntPtr hwnd, int index);
  [DllImport("user32.dll")] static extern IntPtr MonitorFromWindow(IntPtr hwnd, uint flags);
  [DllImport("user32.dll")] static extern bool GetMonitorInfo(IntPtr monitor, ref MonitorInfo info);
  [DllImport("user32.dll")] static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] static extern bool PrintWindow(IntPtr hwnd, IntPtr dc, uint flags);
  public static void Capture(long handle, string output) {
    IntPtr hwnd = new IntPtr(handle); Rect rect; GetWindowRect(hwnd, out rect);
    using (var bitmap = new System.Drawing.Bitmap(rect.right-rect.left, rect.bottom-rect.top)) {
      using (var graphics = System.Drawing.Graphics.FromImage(bitmap)) {
        IntPtr dc = graphics.GetHdc();
        try { if (!PrintWindow(hwnd, dc, 2)) throw new Exception("PrintWindow failed"); }
        finally { graphics.ReleaseHdc(dc); }
      }
      bitmap.Save(output, System.Drawing.Imaging.ImageFormat.Png);
    }
  }
  public static object[] Windows(int owner, string wantedClass, string action) {
    SetProcessDPIAware();
    var result = new List<object>();
    EnumWindows((hwnd, parameter) => {
      uint process; GetWindowThreadProcessId(hwnd, out process);
      if (process != owner) return true;
      var cls = new StringBuilder(512); GetClassName(hwnd, cls, cls.Capacity);
      if (!String.IsNullOrEmpty(wantedClass) && cls.ToString() != wantedClass) return true;
      if (action == "Minimize") ShowWindow(hwnd, 6);
      if (action == "Restore") { ShowWindow(hwnd, 9); SetForegroundWindow(hwnd); }
      if (action == "Close") PostMessage(hwnd, 0x0010, IntPtr.Zero, IntPtr.Zero);
      var title = new StringBuilder(1024); GetWindowText(hwnd, title, title.Capacity);
      Rect rect, client; GetWindowRect(hwnd, out rect); GetClientRect(hwnd, out client);
      var info = new MonitorInfo(); info.size = Marshal.SizeOf(info); GetMonitorInfo(MonitorFromWindow(hwnd, 2), ref info);
      result.Add(new { hwnd=hwnd.ToInt64(), pid=process, class_name=cls.ToString(), title=title.ToString(), rect=rect, client=client,
          monitor=info.monitor, work=info.work, minimized=IsIconic(hwnd), visible=IsWindowVisible(hwnd),
          foreground=GetForegroundWindow()==hwnd, style=GetWindowLongPtr(hwnd,-16).ToInt64() & 0xffffffffL,
          exstyle=GetWindowLongPtr(hwnd,-20).ToInt64() & 0xffffffffL });
      return true;
    }, IntPtr.Zero);
    return result.ToArray();
  }
}
'@
$items = [YingXiaWindowTest]::Windows($OwnerPid, $ClassName, $Action)
if ($Action -eq 'CaptureFrame') {
    if (@($items).Count -ne 1 -or !$OutputPath) { throw 'Capture requires exactly one owned main HWND and an output path' }
    [YingXiaWindowTest]::Capture($items[0].hwnd, $OutputPath)
}
@{windows=@($items)} | ConvertTo-Json -Depth 8 -Compress
