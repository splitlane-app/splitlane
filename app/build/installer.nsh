; SplitLane: what the installer and the uninstaller add to the NSIS templates of electron-builder (spec 3 of
; stage 2). electron-builder includes this file in both builds: the installer and, with BUILD_UNINSTALLER, the
; uninstaller. The installer runs elevated from the start (perMachine); the uninstaller elevates itself in
; un.onInit. The helper's subcommands print progress lines and end with <CMD>-OK or <CMD>-REFUSED: <reason>
; (exit code 0, else 7; 2 is a usage error); uninstall --purge also prints UNINSTALL-NOTE: ...
;
; Installer, in the order of installSection.nsh:
;   customCheckAppRunning (before the uninstaller of an earlier version and before any file is extracted):
;     the window is closed (splitlaneCloseWindow); then splitlane-helper.exe of this version, from dist\helper
;     at build time, runs from $PLUGINSDIR as `prepare --session-user`: it stops the helper of the earlier
;     version with the exit node given back and waits until its processes are gone; a refusal stops the setup
;     (exit code 2) with nothing changed; then the six files of the stage 1 layout leave the root of $INSTDIR.
;   customInstall (after the files): `helper\splitlane-helper.exe install --session-user` in place.
; Uninstaller:
;   customUnWelcomePage: the page with the check box that adds --purge (silent: the parameter /PURGE).
;   customCheckAppRunning: the window is closed (no prepare: the helper's uninstall stops the helper itself).
;   customUnInstall (before the files are removed; nothing when a newer installer runs it to replace the
;     files): `helper\splitlane-helper.exe uninstall [--purge]`; a refusal stops the removal, files stay.
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
  ; "1" when the data folder and the user's settings go too (--purge)
  Var splitlanePurge
  Var splitlanePurgeBox
!else
  !insertmacro splitlaneLineFunction ""
!endif

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
  ${EndIf}
!macroend

!ifndef BUILD_UNINSTALLER
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
      Push $1
      Push ""
      Call splitlaneLine
      Pop $1
      ${If} $1 == ""
        StrCpy $1 "splitlane-helper.exe prepare: $0"
      ${EndIf}
      MessageBox MB_OK|MB_ICONSTOP "SplitLane не установлен: $1" /SD IDOK
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
  ${If} $0 != 0
    Push $1
    Push ""
    Call splitlaneLine
    Pop $1
    ${If} $1 == ""
      StrCpy $1 "splitlane-helper.exe install: $0"
    ${EndIf}
    MessageBox MB_OK|MB_ICONSTOP "SplitLane не установлен: $1" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
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
    nsExec::ExecToStack `"$INSTDIR\helper\splitlane-helper.exe" uninstall$2`
    Pop $0
    Pop $1
    ${If} $0 != 0
      Push $1
      Push ""
      Call un.splitlaneLine
      Pop $1
      ${If} $1 == ""
        StrCpy $1 "splitlane-helper.exe uninstall: $0"
      ${EndIf}
      MessageBox MB_OK|MB_ICONSTOP "SplitLane не удалён: $1" /SD IDOK
      SetErrorLevel 2
      Abort
    ${EndIf}
    ${If} $splitlanePurge == "1"
      Push $1
      Push "UNINSTALL-NOTE:"
      Call un.splitlaneLine
      Pop $1
      ${If} $1 != ""
        MessageBox MB_OK|MB_ICONINFORMATION "$1" /SD IDOK
      ${EndIf}
    ${EndIf}
  ${EndIf}
!macroend
