; UNSIGNED Windows installer for the crofty CLI.
; Installs crofty.exe per-user into %LOCALAPPDATA%\crofty\bin (no admin) and adds
; that folder to the user PATH, so `crofty` works right after install. This is
; the double-click fallback for when an AI agent can't install crofty over the
; shell itself; a human runs it instead.
;
; It also carries Hugo, which crofty wraps, so the author needs no prerequisite.
; hugo.exe sits beside crofty.exe: this directory belongs to crofty alone, so
; nothing of the author's is displaced by putting it there. crofty runs this copy
; rather than searching PATH -- see internal/hugobin.
;
; Unsigned by choice: Windows SmartScreen warns on first run ("Windows protected
; your PC" -> More info -> Run anyway). Code signing is not done.

Unicode true
!include "LogicLib.nsh"
!include "WinMessages.nsh"
!include "FileFunc.nsh"
!include "StrFunc.nsh"

; StrFunc's functions have to be declared before they are used, and the
; uninstaller needs its own copy (the un. variant). StrRep is what takes the
; install dir back out of the user PATH.
${UnStrRep}

; Where Windows looks for "Apps & features" entries. HKCU, not HKLM: this is a
; per-user install (RequestExecutionLevel user), so the entry belongs to the
; user's own hive and needs no admin either.
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\crofty"

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef CROFTY_EXE
  !define CROFTY_EXE "crofty.exe"
!endif
!ifndef HUGO_EXE
  !define HUGO_EXE "hugo.exe"
!endif
!ifndef HUGO_LICENSE
  !define HUGO_LICENSE "..\hugo\LICENSE-hugo.txt"
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
  File "/oname=hugo.exe" "${HUGO_EXE}"
  File "/oname=LICENSE-hugo.txt" "${HUGO_LICENSE}"
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

  ; Register with "Apps & features". Writing the uninstaller to disk is not
  ; enough: without this key nothing lists crofty, and the person this installer
  ; exists for -- the one who never opens a terminal -- can install but not
  ; remove. The key name is fixed, so re-installing (same version or another)
  ; overwrites this entry instead of stacking a second one.
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "crofty"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "ShiroDoromoto"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\crofty.exe"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "https://crofty.site"
  ; Nothing here to modify or repair -- the way to a newer crofty is `crofty
  ; update` or this installer again, so hide the buttons that would lie.
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  ; The size the list shows, in KB. Most of it is the bundled Hugo, and an entry
  ; that claims no size for ~60MB on disk reads as a broken one.
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" $0
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\crofty.exe"
  ; `crofty update` renames the running exe aside before dropping the new one in
  ; (internal/cli/updatecmd.go); the leftover is cleared best-effort there, so it
  ; can still be here, and a leftover file keeps $INSTDIR from going away.
  Delete "$INSTDIR\crofty.exe.old"
  Delete "$INSTDIR\hugo.exe"
  Delete "$INSTDIR\LICENSE-hugo.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  ; %LOCALAPPDATA%\crofty holds nothing but bin\, so take it too. RMDir only
  ; removes an empty directory, so anything else living there is left alone.
  RMDir "$INSTDIR\.."

  ; Take the install dir back out of the user PATH -- but only if this installer
  ; is the one that put it there. The entry is removed with its separators, by
  ; matching ";<dir>;" inside a temporarily fenced string, so a neighbour whose
  ; path merely starts with ours (...\crofty\bin2) survives untouched.
  ReadRegDWORD $3 HKCU "Software\crofty" "PathAdded"
  ${If} $3 == 1
    ReadRegStr $0 HKCU "Environment" "Path"
    StrCpy $0 ";$0;"
    ${UnStrRep} $0 "$0" ";$INSTDIR;" ";"
    StrCpy $0 $0 "" 1   ; drop the leading fence
    StrCpy $0 $0 -1     ; drop the trailing fence
    WriteRegExpandStr HKCU "Environment" "Path" "$0"
    SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  ${EndIf}
  DeleteRegValue HKCU "Software\crofty" "PathAdded"
  DeleteRegKey /ifempty HKCU "Software\crofty"
  DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd
