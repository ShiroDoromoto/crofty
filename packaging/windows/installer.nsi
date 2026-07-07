; UNSIGNED Windows installer for the crofty CLI.
; Installs crofty.exe per-user into %LOCALAPPDATA%\crofty\bin (no admin) and adds
; that folder to the user PATH, so `crofty` works right after install. This is
; the double-click fallback for when an AI agent can't install crofty over the
; shell itself; a human runs it instead.
;
; Unsigned by choice: Windows SmartScreen warns on first run ("Windows protected
; your PC" -> More info -> Run anyway). Code signing is not done.

Unicode true
!include "LogicLib.nsh"
!include "WinMessages.nsh"

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef CROFTY_EXE
  !define CROFTY_EXE "crofty.exe"
!endif
!ifndef OUTFILE
  !define OUTFILE "crofty-setup.exe"
!endif

Name "crofty ${VERSION}"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\crofty\bin"
ShowInstDetails show
BrandingText "crofty ${VERSION}"

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath "$INSTDIR"
  File "/oname=crofty.exe" "${CROFTY_EXE}"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add the install dir to the per-user PATH (HKCU — no admin). A self-set marker
  ; keeps re-installs from appending duplicate entries.
  ReadRegDWORD $3 HKCU "Software\crofty" "PathAdded"
  ${If} $3 != 1
    ReadRegStr $0 HKCU "Environment" "Path"
    ${If} $0 == ""
      WriteRegExpandStr HKCU "Environment" "Path" "$INSTDIR"
    ${Else}
      WriteRegExpandStr HKCU "Environment" "Path" "$0;$INSTDIR"
    ${EndIf}
    WriteRegDWORD HKCU "Software\crofty" "PathAdded" 1
    SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  ${EndIf}
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\crofty.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegValue HKCU "Software\crofty" "PathAdded"
  DeleteRegKey /ifempty HKCU "Software\crofty"
SectionEnd
