' Phlo.vbs starts Phlo when Windows will not run Phlo.cmd.
' It opens the Phlo window when bin\phlo-panel.exe is still there.
' When security software has removed that file, it starts bin\phlo.exe
' and opens the dashboard in the browser.
Option Explicit

Dim shell, fso, root, panel, exe, port, url, i
Set shell = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
root = fso.GetParentFolderName(WScript.ScriptFullName)
shell.CurrentDirectory = root

If Not fso.FileExists(root & "\config\phlo.json") Then
  If fso.FileExists(root & "\config\phlo.json.example") Then
    fso.CopyFile root & "\config\phlo.json.example", root & "\config\phlo.json", True
  End If
End If

EnsureDesktopIcon root, fso, shell
panel = root & "\bin\phlo-panel.exe"
exe = root & "\bin\phlo.exe"
If fso.FileExists(panel) Then
  shell.Run """" & panel & """", 1, False
  WScript.Quit 0
End If

If Not fso.FileExists(exe) Then
  MsgBox "Phlo is not in this folder." & vbCrLf & "Install Go from https://go.dev/dl/ and double-click Phlo.cmd, or build bin\phlo.exe.", vbCritical, "Phlo"
  WScript.Quit 1
End If

port = DashboardPort(root & "\config\phlo.json")
url = "http://127.0.0.1:" & port & "/"
If Healthy(url & "api/health") Then
  shell.Run url, 1, False
  WScript.Quit 0
End If

shell.Run """" & exe & """ start", 1, False
For i = 1 To 30
  WScript.Sleep 1000
  If Healthy(url & "api/health") Then
    shell.Run url, 1, False
    WScript.Quit 0
  End If
Next
MsgBox "Phlo did not open." & vbCrLf & "Leave the Phlo window open, then open " & url, vbExclamation, "Phlo"

Sub EnsureDesktopIcon(root, fso, shell)
  On Error Resume Next
  Dim desktop, target, icon, link
  desktop = shell.SpecialFolders("Desktop")
  If desktop = "" Then Exit Sub
  target = root & "\bin\phlo-panel.exe"
  If Not fso.FileExists(target) Then target = root & "\Phlo.vbs"
  icon = root & "\ui\logo.ico"
  If Not fso.FileExists(icon) Then Exit Sub
  Set link = shell.CreateShortcut(desktop & "\Phlo.lnk")
  link.TargetPath = target
  link.WorkingDirectory = root
  link.IconLocation = icon & ",0"
  link.Description = "Phlo"
  link.WindowStyle = 1
  link.Save
End Sub

Function DashboardPort(path)
  DashboardPort = 7700
  If Not fso.FileExists(path) Then Exit Function
  Dim text, re, matches
  text = fso.OpenTextFile(path, 1).ReadAll()
  Set re = New RegExp
  re.Pattern = """dashboardPort""\s*:\s*(\d+)"
  Set matches = re.Execute(text)
  If matches.Count = 1 Then
    If CLng(matches(0).SubMatches(0)) > 0 Then DashboardPort = CLng(matches(0).SubMatches(0))
  End If
End Function

Function Healthy(addr)
  On Error Resume Next
  Dim http
  Set http = CreateObject("MSXML2.ServerXMLHTTP.6.0")
  If Err.Number <> 0 Then
    Err.Clear
    Set http = CreateObject("MSXML2.XMLHTTP")
  End If
  http.setTimeouts 1000, 1000, 1000, 1000
  http.Open "GET", addr, False
  http.Send
  Healthy = (Err.Number = 0 And http.Status = 200)
  Err.Clear
End Function
