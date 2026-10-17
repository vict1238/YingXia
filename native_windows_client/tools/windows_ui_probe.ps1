param(
  [int]$OwnerPid,
  [switch]$Server,
  [string]$ClassName = 'YingXiaNativeWindow',
  [ValidateSet('Snapshot','Accessibility','Invoke','Click','RealClick','ObservedClick','SetText','Minimize','Restore','Maximize','Escape','Close','Capture')][string]$Action = 'Snapshot',
  [string]$ElementName,
  [string]$Value,
  [string]$OutputPath
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = [Text.UTF8Encoding]::new($false)
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
  [StructLayout(LayoutKind.Sequential)] public struct Msg { public IntPtr hwnd;public uint message;public IntPtr wparam,lparam;public uint time;public int x,y;public uint privateValue; }
  [StructLayout(LayoutKind.Sequential)] public struct Point { public int x,y; }
  [StructLayout(LayoutKind.Sequential)] public struct GuiThreadInfo { public uint size,flags;public IntPtr active,focus,capture,menuOwner,moveSize,caret;public Rect caretRect; }
  [StructLayout(LayoutKind.Sequential)] public struct MouseInput { public int dx,dy;public uint data,flags,time;public UIntPtr extra; }
  [StructLayout(LayoutKind.Explicit)] public struct Input { [FieldOffset(0)]public uint type;[FieldOffset(8)]public MouseInput mouse; }
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc callback,IntPtr param);
  [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent,EnumProc callback,IntPtr param);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hwnd,out uint pid);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr hwnd,StringBuilder text,int count);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr hwnd,StringBuilder text,int count);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hwnd,int command);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint from,uint to,bool attach);
  [DllImport("user32.dll")] public static extern IntPtr SetFocus(IntPtr hwnd);
  [DllImport("user32.dll")] public static extern bool PeekMessage(out Msg msg,IntPtr hwnd,uint min,uint max,uint remove);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr hwnd,IntPtr after,int x,int y,int width,int height,uint flags);
  [DllImport("user32.dll")] public static extern int GetWindowLong(IntPtr hwnd,int index);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out Point point);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(Point point);
  [DllImport("user32.dll")] public static extern uint SendInput(uint count,Input[] input,int size);
  [DllImport("user32.dll")] public static extern bool GetGUIThreadInfo(uint thread,ref GuiThreadInfo info);
  [DllImport("user32.dll")] public static extern bool IsChild(IntPtr parent,IntPtr child);
  public static bool activationUsedPointer=false;
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hwnd,uint message,IntPtr wparam,IntPtr lparam);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr hwnd,uint message,IntPtr wparam,IntPtr lparam);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hwnd,out Rect rect);
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hwnd,IntPtr dc,uint flags);
  public static IntPtr Find(int owner,string cls) {
    IntPtr found=IntPtr.Zero;
    EnumWindows((hwnd,param)=>{uint pid;GetWindowThreadProcessId(hwnd,out pid);var text=new StringBuilder(512);GetClassName(hwnd,text,512);if(pid==owner&&text.ToString()==cls){found=hwnd;return false;}return true;},IntPtr.Zero);
    return found;
  }
  public static string Title(IntPtr hwnd){var text=new StringBuilder(512);GetWindowText(hwnd,text,512);return text.ToString();}
  public static bool OwnsForeground(IntPtr hwnd){uint target,foreground;GetWindowThreadProcessId(hwnd,out target);GetWindowThreadProcessId(GetForegroundWindow(),out foreground);return target!=0&&target==foreground;}
  public static void FocusOwnedView(IntPtr hwnd){
    var view=FlutterView(hwnd);if(view==IntPtr.Zero)throw new Exception("Owned Flutter view is missing");
    uint owner;uint target=GetWindowThreadProcessId(view,out owner),current=GetCurrentThreadId();Msg message;PeekMessage(out message,IntPtr.Zero,0,0,0);bool attached=false;
    try{if(target!=current)attached=AttachThreadInput(current,target,true);SetFocus(view);}finally{if(attached)AttachThreadInput(current,target,false);}
    for(int i=0;i<12;i++){var info=new GuiThreadInfo{size=(uint)Marshal.SizeOf(typeof(GuiThreadInfo))};if(GetGUIThreadInfo(target,ref info)&&(info.focus==view||IsChild(view,info.focus)))return;System.Threading.Thread.Sleep(50);}
    throw new Exception("Owned Flutter view did not receive native input focus");
  }
  public static void Activate(IntPtr hwnd){
    if(Environment.GetEnvironmentVariable("YINGXIA_QA_REQUIRE_ALREADY_FOREGROUND")=="1"&&!OwnsForeground(hwnd))throw new Exception("External foreground interference after owned test activation; no new activation attempt made");
    if(IsIconic(hwnd))ShowWindow(hwnd,9);
    SetForegroundWindow(hwnd);
    if(!OwnsForeground(hwnd)){
      uint ignored;uint foreground=GetWindowThreadProcessId(GetForegroundWindow(),out ignored);
      uint current=GetCurrentThreadId(),target=GetWindowThreadProcessId(hwnd,out ignored);bool attached=false,attachedTarget=false;
      Msg message;PeekMessage(out message,IntPtr.Zero,0,0,0);
      try{if(foreground!=0&&foreground!=current)attached=AttachThreadInput(current,foreground,true);if(target!=0&&target!=current&&target!=foreground)attachedTarget=AttachThreadInput(current,target,true);SetForegroundWindow(hwnd);var view=FlutterView(hwnd);if(view!=IntPtr.Zero)SetFocus(view);}
      finally{if(attachedTarget)AttachThreadInput(current,target,false);if(attached)AttachThreadInput(current,foreground,false);}
    }
    for(int i=0;i<12&&!OwnsForeground(hwnd);i++)System.Threading.Thread.Sleep(50);
    if(!OwnsForeground(hwnd)&&Environment.GetEnvironmentVariable("YINGXIA_QA_REAL_FOCUS_INPUT")=="1"){
      bool wasTopmost=(GetWindowLong(hwnd,-20)&8)!=0;Point cursor;GetCursorPos(out cursor);
      try{
        SetWindowPos(hwnd,new IntPtr(-1),0,0,0,0,0x13);
        Rect rect;GetWindowRect(hwnd,out rect);Point point=new Point{x=rect.left+Math.Min(80,(rect.right-rect.left)/2),y=rect.top+15};
        SetCursorPos(point.x,point.y);uint target,pointOwner;GetWindowThreadProcessId(hwnd,out target);GetWindowThreadProcessId(WindowFromPoint(point),out pointOwner);
        if(target==0||target!=pointOwner)throw new Exception("Observed title point is not owned by the isolated native app; no pointer click sent");
        var down=new Input{type=0,mouse=new MouseInput{flags=2}};var up=new Input{type=0,mouse=new MouseInput{flags=4}};
        if(SendInput(2,new Input[]{down,up},Marshal.SizeOf(typeof(Input)))!=2)throw new Exception("Owned title pointer activation failed");
        activationUsedPointer=true;
        for(int i=0;i<12&&!OwnsForeground(hwnd);i++)System.Threading.Thread.Sleep(50);
      }finally{if(!wasTopmost)SetWindowPos(hwnd,new IntPtr(-2),0,0,0,0,0x13);SetCursorPos(cursor.x,cursor.y);}
    }
    if(!OwnsForeground(hwnd)){uint target,foreground;GetWindowThreadProcessId(hwnd,out target);GetWindowThreadProcessId(GetForegroundWindow(),out foreground);throw new Exception("Owned native test window did not become foreground: target PID="+target+" foreground PID="+foreground);}
    FocusOwnedView(hwnd);
  }
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
  public int role,state,x,y,width,height;
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
    try{node.state=Convert.ToInt32(accessible.get_accState(child));}catch{}
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
    ClickPoint(view,x,y);
  }
  public static void ClickPoint(IntPtr view,int x,int y){
    YingXiaNativeProbe.Rect rect;YingXiaNativeProbe.GetWindowRect(view,out rect);
    if(x<0||y<0||x>=rect.right-rect.left||y>=rect.bottom-rect.top)throw new Exception("Observed target is outside the owned Flutter view");
    var point=new IntPtr((y<<16)|(x&0xffff));
    YingXiaNativeProbe.SendMessage(view,0x0200,IntPtr.Zero,point);
    YingXiaNativeProbe.SendMessage(view,0x0201,new IntPtr(1),point);
    System.Threading.Thread.Sleep(40);
    YingXiaNativeProbe.SendMessage(view,0x0202,IntPtr.Zero,point);
  }
  public static void RealClick(IntPtr view,YingXiaAccessibleNode node){
    YingXiaNativeProbe.Point old;YingXiaNativeProbe.GetCursorPos(out old);
    var point=new YingXiaNativeProbe.Point{x=node.x+node.width/2,y=node.y+node.height/2};
    uint owner,target;YingXiaNativeProbe.GetWindowThreadProcessId(view,out owner);
    try{
      YingXiaNativeProbe.SetCursorPos(point.x,point.y);YingXiaNativeProbe.GetWindowThreadProcessId(YingXiaNativeProbe.WindowFromPoint(point),out target);
      if(owner==0||target!=owner)throw new Exception("Actual current control point is not owned by the isolated app; no click sent");
      var down=new YingXiaNativeProbe.Input{type=0,mouse=new YingXiaNativeProbe.MouseInput{flags=2}};var up=new YingXiaNativeProbe.Input{type=0,mouse=new YingXiaNativeProbe.MouseInput{flags=4}};
      if(YingXiaNativeProbe.SendInput(2,new YingXiaNativeProbe.Input[]{down,up},Marshal.SizeOf(typeof(YingXiaNativeProbe.Input)))!=2)throw new Exception("Actual owned control pointer click failed");
      System.Threading.Thread.Sleep(90);
    }finally{YingXiaNativeProbe.SetCursorPos(old.x,old.y);}
  }
}
'@
function Invoke-YingXiaOwnedCommand {
param([int]$OwnerPid,[string]$ClassName,[string]$Action,[string]$ElementName,[string]$Value,[string]$OutputPath)
if($OwnerPid -le 0){throw 'Provide a positive isolated app owner PID.'}
[YingXiaNativeProbe]::activationUsedPointer=$false
# Only this QA helper thread uses physical coordinate awareness.
# This does not change system settings or the application's DPI configuration.
[void][YingXiaNativeProbe]::SetThreadDpiAwarenessContext([IntPtr](-4))
$window = [YingXiaNativeProbe]::Find($OwnerPid,$ClassName)
if($window -eq [IntPtr]::Zero){throw 'The isolated native app HWND was not found.'}
if($Action -in @('Invoke','Click','RealClick','ObservedClick','SetText','Escape')){
  [YingXiaNativeProbe]::Activate($window)
  Start-Sleep -Milliseconds 180
}
switch($Action){
  'Minimize' {[void][YingXiaNativeProbe]::ShowWindow($window,6)}
  'Restore' {[void][YingXiaNativeProbe]::ShowWindow($window,9);[YingXiaNativeProbe]::Activate($window)}
  'Maximize' {[void][YingXiaNativeProbe]::ShowWindow($window,3);[void][YingXiaNativeProbe]::SetForegroundWindow($window)}
  'Escape' {
    $view=[YingXiaNativeProbe]::FlutterView($window)
    if($view -eq [IntPtr]::Zero){throw 'The native Flutter child view was not found.'}
    [void][YingXiaNativeProbe]::SendMessage($view,0x0100,[IntPtr]27,[IntPtr]0x00010001)
    [void][YingXiaNativeProbe]::SendMessage($view,0x0101,[IntPtr]27,[IntPtr]([long]0xc0010001))
  }
  'Close' {[void][YingXiaNativeProbe]::PostMessage($window,0x0010,[IntPtr]::Zero,[IntPtr]::Zero)}
  'ObservedClick' {
    if($Value -notmatch '^([0-9]+),([0-9]+)$'){throw 'Provide an observed physical screen coordinate.'}
    $pointX=[int]$Matches[1];$pointY=[int]$Matches[2]
    $view=[YingXiaNativeProbe]::FlutterView($window)
    if($view -eq [IntPtr]::Zero){throw 'The native Flutter child view was not found.'}
    $viewBounds=[YingXiaNativeProbe+Rect]::new();[void][YingXiaNativeProbe]::GetWindowRect($view,[ref]$viewBounds)
    [YingXiaMsaaProbe]::ClickPoint($view,($pointX-$viewBounds.left),($pointY-$viewBounds.top))
  }
  'Capture' {
    if(!$OutputPath -or ![IO.Path]::IsPathRooted($OutputPath) -or [IO.Path]::GetFullPath($OutputPath) -ne $OutputPath){throw 'Provide a fully qualified screenshot path.'}
    [YingXiaNativeProbe]::Capture($window,$OutputPath)
  }
}
if($Action -in @('Accessibility','Invoke','Click','RealClick','SetText')){
  $view=[YingXiaNativeProbe]::FlutterView($window)
  if($view -eq [IntPtr]::Zero){throw 'The native Flutter child view was not found.'}
  $nodes=[YingXiaMsaaProbe]::Nodes($view)
  if($nodes.Count -lt 3){Start-Sleep -Milliseconds 300;$nodes=[YingXiaMsaaProbe]::Nodes($view)}
  if($Action -eq 'Accessibility'){
    @{pid=$OwnerPid;hwnd=$window.ToInt64();elements=@($nodes | ForEach-Object{@{name=$_.name;value=$_.value;action=$_.action;role=$_.role;state=$_.state;rect=@($_.x,$_.y,$_.width,$_.height)}})} | ConvertTo-Json -Depth 6 -Compress
    return
  }
  $targets=@($nodes | Where-Object {$_.name -eq $ElementName -or ($_.name -and ($_.name -split "`n")[0] -eq $ElementName)})
  if($Action -eq 'Invoke'){
    $targets=@($targets | Where-Object {$_.role -eq 43 -or $_.action})
    if($targets.Count -ne 1){throw 'Expected one MSAA action target in the isolated app.'}
    $targets[0].accessible.accDoDefaultAction($targets[0].child)
  } elseif($Action -in @('Click','RealClick')) {
    if($targets.Count -gt 1){
      $actionTargets=@($targets | Where-Object {$_.role -eq 43 -or $_.role -eq 46 -or $_.action})
      if($actionTargets.Count -eq 1){$targets=$actionTargets}
    }
    if($targets.Count -ne 1){throw 'Expected one observed target in the isolated app.'}
    if($Action -eq 'RealClick'){[YingXiaMsaaProbe]::RealClick($view,$targets[0])}else{[YingXiaMsaaProbe]::Click($view,$targets[0])}
  } else {
    $targets=@($targets | Where-Object {$_.role -eq 42})
    if($targets.Count -ne 1){throw 'Expected one MSAA text field in the isolated app.'}
    [YingXiaMsaaProbe]::SetValue($view,$targets[0],$Value)
  }
  @{pid=$OwnerPid;action=$Action;name=$ElementName;foreground_activation_used_owned_pointer=[YingXiaNativeProbe]::activationUsedPointer} | ConvertTo-Json -Compress
  return
}
$bounds=[YingXiaNativeProbe+Rect]::new();[void][YingXiaNativeProbe]::GetWindowRect($window,[ref]$bounds)
@{pid=$OwnerPid;hwnd=$window.ToInt64();title=[YingXiaNativeProbe]::Title($window);visible=[YingXiaNativeProbe]::IsWindowVisible($window);minimized=[YingXiaNativeProbe]::IsIconic($window);foreground=[YingXiaNativeProbe]::OwnsForeground($window);foreground_activation_used_owned_pointer=[YingXiaNativeProbe]::activationUsedPointer;rect=@($bounds.left,$bounds.top,$bounds.right,$bounds.bottom);action=$Action} | ConvertTo-Json -Depth 4 -Compress
}
if($Server){
  [Console]::WriteLine('{"server_ready":true}')
  while($line=[Console]::ReadLine()){
    try{
      $request=$line | ConvertFrom-Json
      if($request.Action -eq 'Exit'){break}
      [Environment]::SetEnvironmentVariable('YINGXIA_QA_REQUIRE_ALREADY_FOREGROUND',[string]$request.RequireAlreadyForeground)
      $answer=Invoke-YingXiaOwnedCommand -OwnerPid $request.OwnerPid -ClassName $request.ClassName -Action $request.Action -ElementName $request.ElementName -Value $request.Value -OutputPath $request.OutputPath
      [Console]::WriteLine((@{ok=$true;result=($answer | ConvertFrom-Json)} | ConvertTo-Json -Depth 8 -Compress))
    }catch{[Console]::WriteLine((@{ok=$false;error=$_.Exception.Message} | ConvertTo-Json -Depth 4 -Compress))}
  }
}else{Invoke-YingXiaOwnedCommand -OwnerPid $OwnerPid -ClassName $ClassName -Action $Action -ElementName $ElementName -Value $Value -OutputPath $OutputPath}
