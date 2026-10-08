using System;
using System.Runtime.InteropServices;
public static class AppWindowEvidence {
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
  [StructLayout(LayoutKind.Sequential)] public struct Rect { public int Left,Top,Right,Bottom; }
  [StructLayout(LayoutKind.Sequential)] public struct Point { public int X,Y; public Point(int x,int y){X=x;Y=y;} }
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h,out Rect r);
  [DllImport("user32.dll")] public static extern uint GetDpiForWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(Point p);
  [DllImport("user32.dll")] public static extern IntPtr GetAncestor(IntPtr h,uint flags);
  [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h,ref Point p);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h,System.Text.StringBuilder text,int size);
  [DllImport("user32.dll",SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr h,uint msg,IntPtr w,IntPtr l,uint flags,uint timeout,out IntPtr result);
  [DllImport("user32.dll")] static extern IntPtr OpenInputDesktop(uint flags,bool inherit,uint access);
  [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr h);
  [DllImport("user32.dll")] static extern IntPtr GetProcessWindowStation();
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern bool GetUserObjectInformation(IntPtr h,int index,System.Text.StringBuilder data,uint size,out uint needed);
  static string ObjectName(IntPtr h){var s=new System.Text.StringBuilder(256);uint n;return GetUserObjectInformation(h,2,s,512,out n)?s.ToString():"";}
  public static string InputDesktop(){var h=OpenInputDesktop(0,false,1);if(h==IntPtr.Zero)return "";try{return ObjectName(GetProcessWindowStation())+"/"+ObjectName(h);}finally{CloseDesktop(h);}}
  [DllImport("wtsapi32.dll")] static extern bool WTSQuerySessionInformation(IntPtr server,int session,int info,out IntPtr buffer,out int bytes);
  [DllImport("wtsapi32.dll")] static extern void WTSFreeMemory(IntPtr buffer);
  public static bool ActiveSession(int session){IntPtr p;int n;if(session==0 || !WTSQuerySessionInformation(IntPtr.Zero,session,8,out p,out n))return false;try{return n>=4 && Marshal.ReadInt32(p)==0;}finally{WTSFreeMemory(p);}}
  public static void Pointer(IntPtr root,IntPtr child,int x,int y,bool click){
    if(GetForegroundWindow()!=root || GetAncestor(child,2)!=root)throw new Exception("window lost focus/ownership");
    var point=new Point(x,y);if(WindowFromPoint(point)!=child)throw new Exception("target occluded or moved");
    if(!SetCursorPos(x,y) || !ScreenToClient(child,ref point))throw new Exception("pointer mapping failed");
    var pos=new IntPtr((point.Y<<16)|(point.X&0xffff));IntPtr result;
    if(SendMessageTimeout(child,0x200,IntPtr.Zero,pos,2,500,out result)==IntPtr.Zero)throw new Exception("owned mouse move timed out");
    if(!click)return;
    if(GetForegroundWindow()!=root || WindowFromPoint(new Point(x,y))!=child)throw new Exception("target lost before click");
    try{if(SendMessageTimeout(child,0x201,new IntPtr(1),pos,2,500,out result)==IntPtr.Zero)throw new Exception("owned mouse down timed out");}
    finally{SendMessageTimeout(child,0x202,IntPtr.Zero,pos,2,500,out result);}
  }
}
