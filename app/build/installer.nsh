; SplitLane: what the installer and the uninstaller add to the NSIS templates of electron-builder (spec 3 of
; stage 2). electron-builder includes this file in both builds: the installer and, with BUILD_UNINSTALLER, the
; uninstaller. The installer runs elevated from the start (perMachine); the uninstaller elevates itself in
; un.onInit. The helper's subcommands print progress lines and end with <CMD>-OK or <CMD>-REFUSED: <reason>
; (exit code 0, else 7; 2 is a usage error); uninstall --purge also prints UNINSTALL-NOTE: ...
;
; Installer, in the order of installer.nsi and installSection.nsh:
;   customInit (.onInit, after initMultiUser): $INSTDIR is %ProgramFiles%\SplitLane whatever /D= or an earlier
;     InstallLocation says (spec 2: the folder is fixed).
;   customCheckAppRunning (before the uninstaller of an earlier version and before any file is extracted):
;     the window is closed (splitlaneCloseWindow); then splitlane-helper.exe of this version, from dist\helper
;     at build time, runs from $PLUGINSDIR as `prepare --session-user`: it stops the helper of the earlier
;     version with the exit node given back and waits until its processes are gone; a refusal stops the setup
;     (exit code 2) with nothing changed, and a window the setup closed is started again; then the six files
;     of the stage 1 layout leave the root of $INSTDIR.
;   customInstall (after the files): `helper\splitlane-helper.exe install --session-user` in place; the copy
;     of the installer that the template puts into %LocalAppData% goes (splitlaneDropInstallerCopy). A refusal
;     stops the setup (exit code 2), and a window the setup closed is started again, of the new files.
;   The result for the window (spec 6 of stage 2 part 2, splitlaneWriteResult): after install, and when
;     prepare or install refuses, %ProgramData%\SplitLane\export\last-install.txt says ok or refused and why.
; Uninstaller:
;   customUnWelcomePage: the page with the check box that adds --purge (silent: the parameter /PURGE).
;   customCheckAppRunning: the window is closed (no prepare: the helper's uninstall stops the helper itself).
;   customUnInstall (before the files are removed; nothing when a newer installer runs it to replace the
;     files): `helper\splitlane-helper.exe uninstall [--purge]`; a refusal stops the removal, files stay, and a
;     window the uninstaller closed is started again. A helper that cannot be started at all (missing,
;     quarantined) does not block the removal: the user is asked (silent: only when no helper and no core
;     runs), then the task goes by schtasks and the data folder stays. Then the window's autostart value and
;     the installer's copy in %LocalAppData% go.
;
; Why not the stock _CHECK_APP_RUNNING of electron-builder 26.18: with PowerShell it finds and kills every
; process whose image lies under $INSTDIR, so the helper and its core (helper\, and the root in the stage 1
; layout) would be killed before prepare or uninstall could stop them with the exit node given back; and it
; misses a window started from another folder (win-unpacked), which would start the helper's task again after
; prepare stopped it. Without PowerShell it kills without /F, which a window that hides to the tray survives.
; splitlaneCloseWindow keeps its prompts and order (ask, close, retry) but looks for SplitLane.exe by name.
;
; UTF-8 with BOM: the wizard's texts are Russian.

!include LogicLib.nsh
!include nsDialogs.nsh

; ${UN}splitlaneLine: Push <output>, Push <prefix>, Call, Pop <line>. With the prefix "" the line is the last
; line of the output that is not empty; else the first line that starts with the prefix. "" when there is none.
!macro splitlaneLineFunction UN
  Function ${UN}splitlaneLine
    Exch $R1 ; the prefix
    Exch
    Exch $R0 ; the output
    Push $R2 ; position
    Push $R3 ; start of the current line
    Push $R4 ; length of the output
    Push $R5 ; the result
    Push $R6 ; a character, then the current line
    Push $R7 ; the start of the current line, as long as the prefix
    StrCpy $R2 0
    StrCpy $R3 0
    StrLen $R4 $R0
    StrCpy $R5 ""
    ${Do}
      StrCpy $R6 $R0 1 $R2 ; "" at the end of the output
      ${If} $R6 == "$\r"
      ${OrIf} $R6 == "$\n"
      ${OrIf} $R6 == ""
        IntOp $R6 $R2 - $R3
        StrCpy $R6 $R0 $R6 $R3
        IntOp $R3 $R2 + 1
        ${If} $R6 != ""
          ${If} $R1 == ""
            StrCpy $R5 $R6
          ${Else}
            StrLen $R7 $R1
            StrCpy $R7 $R6 $R7
            ${If} $R7 == $R1
              StrCpy $R5 $R6
              ${Break}
            ${EndIf}
          ${EndIf}
        ${EndIf}
      ${EndIf}
      IntOp $R2 $R2 + 1
    ${LoopUntil} $R2 > $R4
    StrCpy $R0 $R5
    Pop $R7
    Pop $R6
    Pop $R5
    Pop $R4
    Pop $R3
    Pop $R2
    Exch $R0 ; $R0 back, the result on the stack
    Exch
    Pop $R1
  FunctionEnd
!macroend

!ifdef BUILD_UNINSTALLER
  !insertmacro splitlaneLineFunction "un."
  !define SPLITLANE_UN "un."
  ; "1" when the data folder and the user's settings go too (--purge)
  Var splitlanePurge
  Var splitlanePurgeBox
!else
  !insertmacro splitlaneLineFunction ""
  !define SPLITLANE_UN ""
!endif
; "1" when splitlaneCloseWindow found the window and ended it: a refusal starts it again (splitlaneStartWindow)
Var splitlaneClosed

; $1, the output of a refused subcommand LABEL, becomes the reason for the message box: its last line without
; "<LABEL>-REFUSED: ", or FALLBACK when that leaves nothing. Uses $2 and $3.
!macro splitlaneReason LABEL FALLBACK
  Push $1
  Push ""
  Call ${SPLITLANE_UN}splitlaneLine
  Pop $1
  StrLen $2 "${LABEL}-REFUSED: "
  StrCpy $3 $1 $2
  ${If} $3 == "${LABEL}-REFUSED: "
    StrCpy $1 $1 "" $2
  ${EndIf}
  ${If} $1 == ""
    StrCpy $1 "${FALLBACK}"
  ${EndIf}
!macroend

; $0 is 0 when a process SplitLane.exe other than this one ($1, its PID) runs, in any session and from any folder.
!macro splitlaneFindWindow
  nsExec::Exec `"$SYSDIR\cmd.exe" /C ""$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${APP_EXECUTABLE_FILENAME}" /FI "PID ne $1" /FO CSV /NH | "$SYSDIR\findstr.exe" /B /I /C:"\"${APP_EXECUTABLE_FILENAME}\"""`
  Pop $0
!macroend

; The window (SplitLane.exe and its child processes): ask, then end it; while it is still there, ask to close it by
; hand or stop (exit code 2). The prompts are electron-builder's; an update (--updated) closes it without asking.
!macro splitlaneCloseWindow
  System::Call 'kernel32::GetCurrentProcessId() i .r1'
  !insertmacro splitlaneFindWindow
  ${If} $0 == 0
    ${IfNot} ${isUpdated}
      ${IfNot} ${Cmd} `MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "$(appRunning)" /SD IDOK IDOK`
        SetErrorLevel 1
        Quit
      ${EndIf}
    ${EndIf}
    DetailPrint "$(appClosing)"
    StrCpy $2 0
    ${Do}
      nsExec::Exec `"$SYSDIR\taskkill.exe" /F /IM "${APP_EXECUTABLE_FILENAME}" /FI "PID ne $1"`
      Pop $0
      Sleep 500
      !insertmacro splitlaneFindWindow
      ${If} $0 != 0
        ${Break}
      ${EndIf}
      IntOp $2 $2 + 1
      ${If} $2 >= 3
        ${IfNot} ${Cmd} `MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(appCannotBeClosed)" /SD IDCANCEL IDRETRY`
          SetErrorLevel 2
          Quit
        ${EndIf}
        StrCpy $2 0
      ${EndIf}
    ${Loop}
    StrCpy $splitlaneClosed "1"
  ${EndIf}
!macroend

; After a refusal: the window that splitlaneCloseWindow ended starts again, installed SplitLane.exe as the user of
; the desktop, not elevated (StdUtils.ExecShellAsUser, as the template's StartApp), so that there is a window for the
; helper that keeps running. Uses $0. Not when the window was not running or SplitLane.exe is not in $INSTDIR.
!macro splitlaneStartWindow
  ${If} $splitlaneClosed == "1"
  ${AndIf} ${FileExists} "$INSTDIR\${APP_EXECUTABLE_FILENAME}"
    ${StdUtils.ExecShellAsUser} $0 "$INSTDIR\${APP_EXECUTABLE_FILENAME}" "open" ""
  ${EndIf}
!macroend

; The template copies the running installer into %LocalAppData%\<name>-updater\installer.exe of the elevated user
; (installApplicationFiles in include\installer.nsh, unconditional in app-builder-lib 26.18: no option or define
; turns it off): an elevated write into a folder that user controls, never removed. It goes here, and its folder
; when that is empty (RMDir without /r removes a junction itself, never what it points to). Keeps the registers.
!macro splitlaneDropInstallerCopy
  Push $0
  ${If} $installMode == "all"
    SetShellVarContext current
  ${EndIf}
  Delete "$LOCALAPPDATA\${APP_INSTALLER_STORE_FILE}"
  ${StdUtils.GetParentPath} $0 "$LOCALAPPDATA\${APP_INSTALLER_STORE_FILE}"
  ${If} $0 != ""
    RMDir "$0"
  ${EndIf}
  ${If} $installMode == "all"
    SetShellVarContext all
  ${EndIf}
  Pop $0
!macroend

!ifndef BUILD_UNINSTALLER
  ; splitlaneWriteResult RESULT REASON: %ProgramData%\SplitLane\export\last-install.txt is replaced whole by three
  ; lines, ${VERSION}, RESULT (ok or refused) and REASON (one line, "" for ok); CRLF, UTF-8 without BOM. The window
  ; reads it when it starts again after an update it started. The folder is the helper's install's (administrators
  ; write, the user reads; spec 1, 4.2) and is not made here: without it nothing is written. A write that fails
  ; is no failure of the setup: no message; the registers, the error flag and the shell context stay as they were.
  !macro splitlaneWriteResult RESULT REASON
    Push "${VERSION}$\r$\n${RESULT}$\r$\n${REASON}$\r$\n"
    Call splitlaneWriteResultFile
  !macroend

  ; Push <text>, Call. The text (UTF-16 in NSIS) is converted to UTF-8 with WideCharToMultiByte and written with
  ; WriteFile: FileWrite of the Unicode NSIS converts to the ANSI code page. A file left half written is removed.
  Function splitlaneWriteResultFile
    Exch $R0 ; the text
    Push $R1 ; $APPDATA of the shell context in effect, then the file's handle
    Push $R2 ; the folder
    Push $R3 ; the size of the UTF-8 text, with its NUL, then without
    Push $R4 ; the buffer of the UTF-8 text
    Push $R5 ; "1" when the error flag was set
    Push $R6 ; what WriteFile returns, then "done" when the whole text is written
    Push $R7 ; the bytes written
    StrCpy $R5 ""
    ${If} ${Errors} ; it clears the flag
      StrCpy $R5 "1"
    ${EndIf}
    ; %ProgramData% is the known folder, as for the helper: $APPDATA of the shell context "all". The template sets
    ; "all" in .onInit (perMachine); a context "current" (its $APPDATA differs) is put back at once.
    StrCpy $R1 $APPDATA
    SetShellVarContext all
    StrCpy $R2 "$APPDATA\SplitLane\export"
    ${If} $R1 != $APPDATA
      SetShellVarContext current
    ${EndIf}
    ${If} ${FileExists} "$R2\*.*"
      FileOpen $R1 "$R2\last-install.txt" w
      ${IfNot} ${Errors}
        StrCpy $R6 ""
        System::Call 'kernel32::WideCharToMultiByte(i 65001, i 0, w R0, i -1, p 0, i 0, p 0, p 0) i .R3'
        ${If} $R3 > 1
          System::Alloc $R3
          Pop $R4
          ${If} $R4 != 0
            System::Call 'kernel32::WideCharToMultiByte(i 65001, i 0, w R0, i -1, p R4, i R3, p 0, p 0) i .R3'
            ${If} $R3 > 1
              IntOp $R3 $R3 - 1
              StrCpy $R7 0
              System::Call 'kernel32::WriteFile(p R1, p R4, i R3, *i 0 R7, p 0) i .R6'
              ${If} $R6 != 0
              ${AndIf} $R7 == $R3
                StrCpy $R6 "done"
              ${Else}
                StrCpy $R6 ""
              ${EndIf}
            ${EndIf}
            System::Free $R4
          ${EndIf}
        ${EndIf}
        FileClose $R1
        ${If} $R6 != "done"
          Delete "$R2\last-install.txt"
        ${EndIf}
      ${EndIf}
    ${EndIf}
    ClearErrors
    ${If} $R5 == "1"
      SetErrors
    ${EndIf}
    Pop $R7
    Pop $R6
    Pop $R5
    Pop $R4
    Pop $R3
    Pop $R2
    Pop $R1
    Pop $R0
  FunctionEnd

  ; Spec 3, steps 2 and 3: prepare, then the stage 1 layout leaves the root.
  !macro splitlanePrepare
    ; installSection.nsh hides the details of the wizard (none; both when silent); this step shows its line
    ; above the progress bar while it runs, then the setting is put back
    SetDetailsPrint textonly
    DetailPrint "Подготовка: служебный процесс прежней версии останавливается…"
    ${If} ${Silent}
      SetDetailsPrint both
    ${Else}
      SetDetailsPrint none
    ${EndIf}
    File "/oname=$PLUGINSDIR\splitlane-helper.exe" "${PROJECT_DIR}\..\dist\helper\splitlane-helper.exe"
    nsExec::ExecToStack `"$PLUGINSDIR\splitlane-helper.exe" prepare --session-user`
    Pop $0 ; the exit code, or "error"
    Pop $1 ; the output
    ${If} $0 != 0
      !insertmacro splitlaneReason PREPARE "splitlane-helper.exe prepare: $0"
      ; before the window starts again: it reads the result when it starts
      !insertmacro splitlaneWriteResult refused $1
      MessageBox MB_OK|MB_ICONSTOP "SplitLane не установлен: $1" /SD IDOK
      !insertmacro splitlaneStartWindow
      SetErrorLevel 2
      Quit
    ${EndIf}
    ; the helper of stage 1 (plan 1B) lay at the root; its processes are gone now
    Delete "$INSTDIR\splitlane-helper.exe"
    Delete "$INSTDIR\sing-box.exe"
    Delete "$INSTDIR\libcronet.dll"
    Delete "$INSTDIR\sing-box-LICENSE"
    Delete "$INSTDIR\LICENSE"
    Delete "$INSTDIR\THIRD-PARTY.md"
  !macroend
!endif

!ifndef BUILD_UNINSTALLER
  ; spec 2: the folder is fixed. initMultiUser (just before) takes an earlier InstallLocation or /D=; neither counts.
  !macro customInit
    StrCpy $INSTDIR "$PROGRAMFILES64\SplitLane"
  !macroend
!endif

!macro customCheckAppRunning
  !insertmacro splitlaneCloseWindow
  !ifndef BUILD_UNINSTALLER
    !insertmacro splitlanePrepare
  !endif
!macroend

!macro customInstall
  nsExec::ExecToStack `"$INSTDIR\helper\splitlane-helper.exe" install --session-user`
  Pop $0
  Pop $1
  !insertmacro splitlaneDropInstallerCopy
  ${If} $0 != 0
    !insertmacro splitlaneReason INSTALL "splitlane-helper.exe install: $0"
    !insertmacro splitlaneWriteResult refused $1
    MessageBox MB_OK|MB_ICONSTOP "SplitLane не установлен: $1" /SD IDOK
    ; the window of the new files (Abort skips --force-run): its repair screen and the result of the update
    !insertmacro splitlaneStartWindow
    SetErrorLevel 2
    Abort
  ${EndIf}
  ; before --force-run starts the window
  !insertmacro splitlaneWriteResult ok ""
!macroend

!macro customUnWelcomePage
  Function un.splitlanePurgeShow
    !insertmacro MUI_HEADER_TEXT "$(^UninstallCaption)" ""
    nsDialogs::Create 1018
    Pop $0
    ${If} $0 == error
      Abort
    ${EndIf}
    ${NSD_CreateCheckbox} 0 0 100% 12u "Удалить настройки и вход в Tailscale"
    Pop $splitlanePurgeBox
    ${If} $splitlanePurge == "1"
      ${NSD_Check} $splitlanePurgeBox
    ${EndIf}
    ${NSD_CreateLabel} 0 20u 100% 36u "Без галочки настройки окна и вход встроенного узла в Tailscale остаются для следующей установки."
    Pop $0
    ; the next page removes at once: the button says so
    GetDlgItem $0 $HWNDPARENT 1
    SendMessage $0 ${WM_SETTEXT} 0 "STR:$(^UninstallBtn)"
    nsDialogs::Show
  FunctionEnd

  Function un.splitlanePurgeLeave
    ${NSD_GetState} $splitlanePurgeBox $0
    ${If} $0 == ${BST_CHECKED}
      StrCpy $splitlanePurge "1"
    ${Else}
      StrCpy $splitlanePurge "0"
    ${EndIf}
  FunctionEnd

  UninstPage custom un.splitlanePurgeShow un.splitlanePurgeLeave
!macroend

!macro customUnInstall
  ${IfNot} ${isUpdated}
    ${If} ${Silent}
      ${GetParameters} $0
      ClearErrors
      ${GetOptions} $0 "/PURGE" $1
      ${IfNot} ${Errors}
        StrCpy $splitlanePurge "1"
      ${EndIf}
    ${EndIf}
    ${If} $splitlanePurge == "1"
      StrCpy $2 " --purge"
    ${Else}
      StrCpy $2 ""
    ${EndIf}
    ${If} ${FileExists} "$INSTDIR\helper\splitlane-helper.exe"
      nsExec::ExecToStack `"$INSTDIR\helper\splitlane-helper.exe" uninstall$2`
      Pop $0 ; the exit code, or "error" when it could not be started
      Pop $1 ; the output
    ${Else}
      StrCpy $0 "error"
      StrCpy $1 ""
    ${EndIf}
    ${If} $0 == "error"
      !insertmacro splitlaneUninstallWithoutHelper
    ${ElseIf} $0 != 0
      !insertmacro splitlaneReason UNINSTALL "splitlane-helper.exe uninstall: $0"
      MessageBox MB_OK|MB_ICONSTOP "SplitLane не удалён: $1" /SD IDOK
      !insertmacro splitlaneStartWindow
      SetErrorLevel 2
      Abort
    ${ElseIf} $splitlanePurge == "1"
      Push $1
      Push "UNINSTALL-NOTE: "
      Call un.splitlaneLine
      Pop $1
      StrLen $2 "UNINSTALL-NOTE: "
      StrCpy $1 $1 "" $2
      ${If} $1 != ""
        MessageBox MB_OK|MB_ICONINFORMATION "$1" /SD IDOK
      ${EndIf}
    ${EndIf}
    ; the window's autostart (loginItem.ts names it SplitLane) and the installer's copy of the template
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "SplitLane"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run" "SplitLane"
    !insertmacro splitlaneDropInstallerCopy
  ${EndIf}
!macroend

!ifdef BUILD_UNINSTALLER
  ; $0 is 0 when a process with the image name IMAGE runs, in any session and from any folder.
  !macro splitlaneFindImage IMAGE
    nsExec::Exec `"$SYSDIR\cmd.exe" /C ""$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${IMAGE}" /FO CSV /NH | "$SYSDIR\findstr.exe" /B /I /C:"\"${IMAGE}\"""`
    Pop $0
  !macroend

  ; helper\splitlane-helper.exe cannot be started (missing, or blocked by an antivirus): the uninstall cannot give
  ; the exit node back, but it must not make SplitLane impossible to remove. The user decides (No: the removal
  ; stops, exit code 2); silent, it goes on only while no helper and no core runs. Then the task goes; the data
  ; folder stays (only the helper removes it: it does not follow junctions).
  !macro splitlaneUninstallWithoutHelper
    ${If} ${Silent}
      !insertmacro splitlaneFindImage "splitlane-helper.exe"
      ${If} $0 != 0
        !insertmacro splitlaneFindImage "sing-box.exe"
      ${EndIf}
      ${If} $0 == 0
        !insertmacro splitlaneStartWindow
        SetErrorLevel 2
        Abort
      ${EndIf}
    ${ElseIfNot} ${Cmd} `MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 "Служебный процесс SplitLane не запускается, поэтому удаление не может проверить, что exit node Tailscale включён. Удалить SplitLane всё равно? Если после удаления не работает VPN, включите exit node в приложении Tailscale." IDYES`
      !insertmacro splitlaneStartWindow
      SetErrorLevel 2
      Abort
    ${EndIf}
    ; schtasks of the system folder; a task that is not there ends it with code 1, which is no error here
    nsExec::Exec `"$SYSDIR\schtasks.exe" /Delete /TN "SplitLane Helper" /F`
    Pop $0
    ${If} $splitlanePurge == "1"
      MessageBox MB_OK|MB_ICONINFORMATION "Служебный процесс не запустился, поэтому папка %ProgramData%\SplitLane с данными и входом в Tailscale осталась: её можно удалить вручную. Настройки окна в %AppData%\SplitLane тоже остались." /SD IDOK
    ${EndIf}
  !macroend
!endif
