; The Kivo installer, built by build.ps1:
;   iscc /DVersion=0.3.1 /DStage=<staged files> kivo.iss
; Stage holds kivo.exe, kivow.exe, ffmpeg\, kivo.ico, LICENSE.txt and
; THIRD-PARTY-NOTICES.txt.

#ifndef Version
  #error Pass /DVersion=<version>
#endif
#ifndef Stage
  #error Pass /DStage=<folder of staged files>
#endif

[Setup]
AppId={{E4976562-B581-4E2E-A524-119AC47BEBDD}
AppName=Kivo
AppVersion={#Version}
AppPublisher=Kivo
AppPublisherURL=https://github.com/kivodev/kivo
AppSupportURL=https://github.com/kivodev/kivo/issues
DefaultDirName={autopf}\Kivo
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
LicenseFile={#Stage}\LICENSE.txt
InfoBeforeFile={#Stage}\THIRD-PARTY-NOTICES.txt
SetupIconFile={#Stage}\kivo.ico
UninstallDisplayIcon={app}\kivo.ico
OutputBaseFilename=Kivo-Setup-{#Version}
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
; A running Kivo is asked to close, and stopped if it doesn't, so its files
; can be replaced. The finish page starts it again.
CloseApplications=force
RestartApplications=no
; The Startup shortcut is the installing user's, which is what's meant.
UsedUserAreasWarning=no

[Dirs]
; Kivo runs as whoever signs in, and keeps its data here.
Name: "{commonappdata}\Kivo"; Permissions: users-modify; Flags: uninsneveruninstall

[Files]
Source: "{#Stage}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs

[Icons]
Name: "{autoprograms}\Kivo"; Filename: "{app}\kivow.exe"; IconFilename: "{app}\kivo.ico"
Name: "{userstartup}\Kivo"; Filename: "{app}\kivow.exe"; IconFilename: "{app}\kivo.ico"

[Run]
; Private networks only: the web app and the apps on TCP 8484, and LAN
; discovery on UDP 5353. Old rules go first, so an upgrade doesn't double them.
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Kivo"""; Flags: runhidden
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Kivo"" dir=in action=allow profile=private protocol=TCP localport=8484 program=""{app}\kivow.exe"""; Flags: runhidden
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Kivo"" dir=in action=allow profile=private protocol=UDP localport=5353 program=""{app}\kivow.exe"""; Flags: runhidden
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Kivo"" dir=in action=allow profile=private protocol=TCP localport=8484 program=""{app}\kivo.exe"""; Flags: runhidden
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Kivo"" dir=in action=allow profile=private protocol=UDP localport=5353 program=""{app}\kivo.exe"""; Flags: runhidden
; Not elevated, so Kivo runs as the user and sees their mapped drives.
Filename: "{app}\kivow.exe"; Description: "Launch Kivo"; Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/f /im kivow.exe /im kivo.exe"; Flags: runhidden; RunOnceId: "StopKivo"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Kivo"""; Flags: runhidden; RunOnceId: "DeleteFirewallRules"

[Code]
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usPostUninstall) and not UninstallSilent then
    MsgBox('Your Kivo library, settings and backups are kept in ' + ExpandConstant('{commonappdata}\Kivo') +
      '. Delete that folder to remove them too.', mbInformation, MB_OK);
end;
