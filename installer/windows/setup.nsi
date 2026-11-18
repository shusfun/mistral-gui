Unicode true
!ifndef VERSION
!define VERSION "0.1.0"
!endif
Name "mistral"
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "mistral"
VIAddVersionKey /LANG=1033 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "FileDescription" "mistral installer"
OutFile "..\..\dist\mistral-gui-windows-x64-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\mistral-gui"
InstallDirRegKey HKCU "Software\mistral-gui" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
!include "MUI2.nsh"
!define MUI_ICON "..\..\internal\branding\assets\app.ico"
!define MUI_UNICON "..\..\internal\branding\assets\app.ico"
!include "FileFunc.nsh"
!define MUI_LANGDLL_REGISTRY_ROOT HKCU
!define MUI_LANGDLL_REGISTRY_KEY "Software\mistral-gui"
!define MUI_LANGDLL_REGISTRY_VALUENAME "InstallerLanguage"
!define MUI_LANGDLL_ALWAYSSHOW
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION Launch
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_RESERVEFILE_LANGDLL

LangString ConfirmRemoveData ${LANG_SIMPCHINESE} "是否同时删除本地模型、配置与凭据数据目录（$APPDATA\mistral-gui，约 $0 MB）？$\r$\n选择“否”将保留已下载的模型。"
LangString ConfirmRemoveData ${LANG_ENGLISH} "Also delete local models, settings, and credentials in $APPDATA\mistral-gui (~$0 MB)?$\r$\nChoose 'No' to keep downloaded models."

Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
FunctionEnd
Function Launch
  StrCpy $0 "en-US"
  StrCmp $LANGUAGE 2052 0 +2
  StrCpy $0 "zh-CN"
  Exec '"$INSTDIR\mistral-gui.exe" --language $0'
FunctionEnd
Section "mistral"
  SetShellVarContext current
  IfFileExists "$INSTDIR\mistral-gui.exe" 0 +4
  ExecWait '"$INSTDIR\mistral-gui.exe" --data-dir "$APPDATA\mistral-gui" --quit'
  nsExec::ExecToStack 'taskkill /F /IM mistral-gui.exe'
  Sleep 1000
  SetOutPath "$INSTDIR"
  File "..\..\dist\mistral-gui.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateDirectory "$SMPROGRAMS\mistral"
  IfFileExists "$SMPROGRAMS\mistral\mistral.lnk" +2 0
  CreateShortcut "$SMPROGRAMS\mistral\mistral.lnk" "$INSTDIR\mistral-gui.exe" "" "$INSTDIR\mistral-gui.exe" 0
  IfFileExists "$SMPROGRAMS\mistral\Uninstall.lnk" +2 0
  CreateShortcut "$SMPROGRAMS\mistral\Uninstall.lnk" "$INSTDIR\uninstall.exe"
  IfFileExists "$DESKTOP\mistral.lnk" +2 0
  CreateShortcut "$DESKTOP\mistral.lnk" "$INSTDIR\mistral-gui.exe" "" "$INSTDIR\mistral-gui.exe" 0
  WriteRegStr HKCU "Software\mistral-gui" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "DisplayName" "mistral"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "DisplayIcon" "$INSTDIR\mistral-gui.exe,0"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "NoRepair" 1
SectionEnd
Function un.onInit
  !insertmacro MUI_UNGETLANGUAGE
FunctionEnd
Section "Uninstall"
  SetShellVarContext current
  ; Stop running instance across default or custom port recorded in instance.json, plus fallback taskkill (M-S28)
  IfFileExists "$INSTDIR\mistral-gui.exe" 0 +2
  ExecWait '"$INSTDIR\mistral-gui.exe" --data-dir "$APPDATA\mistral-gui" --quit'
  nsExec::ExecToStack 'taskkill /F /IM mistral-gui.exe'
  Sleep 1000
  Delete "$INSTDIR\mistral-gui.exe"
  Delete "$INSTDIR\mistral-gui.exe.previous"
  Delete "$INSTDIR\mistral-gui.exe.next"
  Delete "$INSTDIR\mistral-gui.exe.failed-*"
  RMDir /r "$INSTDIR\.mistral-gui-unpack"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$DESKTOP\mistral.lnk"
  Delete "$SMPROGRAMS\mistral\mistral.lnk"
  Delete "$SMPROGRAMS\mistral\Uninstall.lnk"
  RMDir "$SMPROGRAMS\mistral"
  Delete "$DESKTOP\mistral-gui.lnk"
  Delete "$SMPROGRAMS\mistral-gui\mistral-gui.lnk"
  Delete "$SMPROGRAMS\mistral-gui\Uninstall.lnk"
  RMDir "$SMPROGRAMS\mistral-gui"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "mistral-gui"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui"
  DeleteRegKey HKCU "Software\mistral-gui"

  ; Prompt interactive user to optionally remove $APPDATA\mistral-gui with disk usage; silent uninstall (/S) defaults to IDNO (M-S20)
  IfFileExists "$APPDATA\mistral-gui\*.*" 0 doneData
  ${GetSize} "$APPDATA\mistral-gui" "/S=0M" $0 $1 $2
  MessageBox MB_YESNO|MB_ICONQUESTION "$(ConfirmRemoveData)" /SD IDNO IDNO doneData
  RMDir /r "$APPDATA\mistral-gui"
  doneData:
SectionEnd
