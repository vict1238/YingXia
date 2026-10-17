param(
  [Parameter(Mandatory=$true)][int]$OwnerPid,
  [string]$ClassName = 'YingXiaNativeWindow',
  [ValidateSet('Snapshot','Accessibility','Invoke','Click','SetText','Minimize','Restore','Close','Capture')][string]$Action = 'Snapshot',
  [string]$ElementName,
  [string]$Value,
  [string]$OutputPath
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$ProgressPreference='SilentlyContinue'
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes,System.Drawing,Accessibility
Add-Type -ReferencedAssemblies System.Drawing,Accessibility -TypeDefinition @'
using System;
using System.Text;
using System.Collections.Generic;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
public static class YingXiaNativeProbe {
  public delegate bool EnumProc(IntPtr hwnd, IntPtr param);
  [StructLayout(LayoutKind.Sequential)] public struct Rect { public int left,top,right,bottom; }
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc callback,IntPtr param);
  [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent,EnumProc callback,IntPtr param);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hwnd,out uint pid);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr hwnd,StringBuilder text,int count);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr hwnd,StringBuilder text,int count);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hwnd,int command);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hwnd,uint message,IntPtr wparam,IntPtr lparam);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr hwnd,uint message,IntPtr wparam,IntPtr lparam);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hwnd,out Rect rect);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hwnd,IntPtr dc,uint flags);
  public static IntPtr Find(int owner,string cls) {
    IntPtr found=IntPtr.Zero;
    EnumWindows((hwnd,param)=>{uint pid;GetWindowThreadProcessId(hwnd,out pid);var text=new StringBuilder(512);GetClassName(hwnd,text,512);if(pid==owner&&text.ToString()==cls){found=hwnd;return false;}return true;},IntPtr.Zero);
    return found;
  }
  public static string Title(IntPtr hwnd){var text=new StringBuilder(512);GetWindowText(hwnd,text,512);return text.ToString();}
  public static IntPtr FlutterView(IntPtr hwnd) {
    IntPtr found=IntPtr.Zero;EnumChildWindows(hwnd,(child,param)=>{var text=new StringBuilder(512);GetClassName(child,text,512);if(text.ToString()=="FLUTTERVIEW"){found=child;return false;}return true;},IntPtr.Zero);return found;
  }
  public static void Capture(IntPtr hwnd,string path) {
    Rect rect;GetWindowRect(hwnd,out rect);
    using(var bitmap=new Bitmap(rect.right-rect.left,rect.bottom-rect.top,PixelFormat.Format32bppArgb)){
      using(var graphics=Graphics.FromImage(bitmap)){var dc=graphics.GetHdc();try{if(!PrintWindow(hwnd,dc,2))throw new Exception("PrintWindow failed");}finally{graphics.ReleaseHdc(dc);}}
      bitmap.Save(path,ImageFormat.Png);
    }
  }
}
public class YingXiaAccessibleNode {
  public string name,value,action;
  public int role,x,y,width,height;
  public Accessibility.IAccessible accessible;
  public object child;
}
public static class YingXiaMsaaProbe {
  [DllImport("oleacc.dll")] static extern int AccessibleObjectFromWindow(IntPtr hwnd,uint objectId,ref Guid iid,[MarshalAs(UnmanagedType.Interface)] out object accessible);
  [DllImport("oleacc.dll")] static extern int AccessibleChildren(Accessibility.IAccessible accessible,int start,int count,[Out,MarshalAs(UnmanagedType.LPArray,SizeParamIndex=2)] object[] children,out int obtained);
  static void Walk(Accessibility.IAccessible accessible,object child,List<YingXiaAccessibleNode> rows,int depth) {
    if(depth>30||rows.Count>2500)return;
    var node=new YingXiaAccessibleNode{accessible=accessible,child=child};
    try{node.name=accessible.get_accName(child);}catch{}
    try{node.value=accessible.get_accValue(child);}catch{}
    try{node.action=accessible.get_accDefaultAction(child);}catch{}
    try{node.role=Convert.ToInt32(accessible.get_accRole(child));}catch{}
    try{accessible.accLocation(out node.x,out node.y,out node.width,out node.height,child);}catch{}
    rows.Add(node);
    if(!(child is int)||Convert.ToInt32(child)!=0)return;
    int count;try{count=accessible.accChildCount;}catch{return;}
    if(count<=0||count>2500)return;
    object[] children=new object[count];int obtained;
    if(AccessibleChildren(accessible,0,count,children,out obtained)<0)return;
    for(int i=0;i<obtained;i++){
      var nested=children[i] as Accessibility.IAccessible;
      if(nested!=null)Walk(nested,0,rows,depth+1);
      else if(children[i] is int)Walk(accessible,children[i],rows,depth+1);
    }
  }
  public static List<YingXiaAccessibleNode> Nodes(IntPtr hwnd) {
    object value;Guid iid=typeof(Accessibility.IAccessible).GUID;
    if(AccessibleObjectFromWindow(hwnd,unchecked((uint)-4),ref iid,out value)<0)return new List<YingXiaAccessibleNode>();
    var root=value as Accessibility.IAccessible;var rows=new List<YingXiaAccessibleNode>();if(root!=null)Walk(root,0,rows,0);return rows;
  }
  public static void SetValue(IntPtr view,YingXiaAccessibleNode node,string value){
    // Stable Flutter exposes text fields through MSAA but does not implement
    // put_accValue. Focus its owned field and use its own WM_CHAR input path.
    node.accessible.accDoDefaultAction(node.child);
    System.Threading.Thread.Sleep(120);
    int length=(node.value??"").Length;
    for(int i=0;i<length;i++){
      YingXiaNativeProbe.SendMessage(view,0x0100,new IntPtr(8),new IntPtr(0x000e0001));
      YingXiaNativeProbe.SendMessage(view,0x0101,new IntPtr(8),new IntPtr(unchecked((int)0xc00e0001)));
      System.Threading.Thread.Sleep(10);
    }
    foreach(char character in value){YingXiaNativeProbe.SendMessage(view,0x0102,new IntPtr(character),new IntPtr(1));System.Threading.Thread.Sleep(5);}
  }
  public static void Click(IntPtr view,YingXiaAccessibleNode node){
    YingXiaNativeProbe.Rect rect;YingXiaNativeProbe.GetWindowRect(view,out rect);
    int x=node.x+node.width/2-rect.left,y=node.y+node.height/2-rect.top;
    if(x<0||y<0||x>=rect.right-rect.left||y>=rect.bottom-rect.top)throw new Exception("Observed target is outside the owned Flutter view");
    var point=new IntPtr((y<<16)|(x&0xffff));
    YingXiaNativeProbe.SendMessage(view,0x0200,IntPtr.Zero,point);
    YingXiaNativeProbe.SendMessage(view,0x0201,new IntPtr(1),point);
    System.Threading.Thread.Sleep(40);
    YingXiaNativeProbe.SendMessage(view,0x0202,IntPtr.Zero,point);
  }
}
'@
$window = [YingXiaNativeProbe]::Find($OwnerPid,$ClassName)
if($window -eq [IntPtr]::Zero){throw 'The isolated native app HWND was not found.'}
switch($Action){
  'Minimize' {[void][YingXiaNativeProbe]::ShowWindow($window,6)}
  'Restore' {[void][YingXiaNativeProbe]::ShowWindow($window,9);[void][YingXiaNativeProbe]::SetForegroundWindow($window)}
  'Close' {[void][YingXiaNativeProbe]::PostMessage($window,0x0010,[IntPtr]::Zero,[IntPtr]::Zero)}
  'Capture' {
    if(!$OutputPath -or ![IO.Path]::IsPathRooted($OutputPath) -or [IO.Path]::GetFullPath($OutputPath) -ne $OutputPath){throw 'Provide a fully qualified screenshot path.'}
    [YingXiaNativeProbe]::Capture($window,$OutputPath)
  }
}
if($Action -in @('Accessibility','Invoke','Click','SetText')){
  $view=[YingXiaNativeProbe]::FlutterView($window)
  if($view -eq [IntPtr]::Zero){throw 'The native Flutter child view was not found.'}
  $nodes=[YingXiaMsaaProbe]::Nodes($view)
  if($nodes.Count -lt 3){Start-Sleep -Milliseconds 300;$nodes=[YingXiaMsaaProbe]::Nodes($view)}
  if($Action -eq 'Accessibility'){
    @{pid=$OwnerPid;hwnd=$window.ToInt64();elements=@($nodes | ForEach-Object{@{name=$_.name;value=$_.value;action=$_.action;role=$_.role;rect=@($_.x,$_.y,$_.width,$_.height)}})} | ConvertTo-Json -Depth 6 -Compress
    exit
  }
  $targets=@($nodes | Where-Object {$_.name -eq $ElementName -or ($_.name -and ($_.name -split "`n")[0] -eq $ElementName)})
  if($Action -eq 'Invoke'){
    $targets=@($targets | Where-Object {$_.role -eq 43 -or $_.action})
    if($targets.Count -ne 1){throw 'Expected one MSAA action target in the isolated app.'}
    $targets[0].accessible.accDoDefaultAction($targets[0].child)
  } elseif($Action -eq 'Click') {
    if($targets.Count -ne 1){throw 'Expected one observed target in the isolated app.'}
    [YingXiaMsaaProbe]::Click($view,$targets[0])
  } else {
    $targets=@($targets | Where-Object {$_.role -eq 42})
    if($targets.Count -ne 1){throw 'Expected one MSAA text field in the isolated app.'}
    [YingXiaMsaaProbe]::SetValue($view,$targets[0],$Value)
  }
  @{pid=$OwnerPid;action=$Action;name=$ElementName} | ConvertTo-Json -Compress
  exit
}
$bounds=[YingXiaNativeProbe+Rect]::new();[void][YingXiaNativeProbe]::GetWindowRect($window,[ref]$bounds)
@{pid=$OwnerPid;hwnd=$window.ToInt64();title=[YingXiaNativeProbe]::Title($window);visible=[YingXiaNativeProbe]::IsWindowVisible($window);minimized=[YingXiaNativeProbe]::IsIconic($window);rect=@($bounds.left,$bounds.top,$bounds.right,$bounds.bottom);action=$Action} | ConvertTo-Json -Depth 4 -Compress
