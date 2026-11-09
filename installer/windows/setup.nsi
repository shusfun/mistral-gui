Unicode true
!ifndef VERSION
!define VERSION "0.1.0"
!endif
Name "mistral-gui"
OutFile "..\..\dist\mistral-gui-windows-x64-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\mistral-gui"
InstallDirRegKey HKCU "Software\mistral-gui" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
!include "MUI2.nsh"
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
Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
FunctionEnd
Function Launch
  StrCpy $0 "en-US"
  StrCmp $LANGUAGE 2052 0 +2
  StrCpy $0 "zh-CN"
  Exec '"$INSTDIR\mistral-gui.exe" --language $0'
FunctionEnd
Section "mistral-gui"
  SetShellVarContext current
  IfFileExists "$INSTDIR\mistral-gui.exe" 0 +3
  ExecWait '"$INSTDIR\mistral-gui.exe" --quit'
  Sleep 1000
  SetOutPath "$INSTDIR"
  File "..\..\dist\mistral-gui.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateDirectory "$SMPROGRAMS\mistral-gui"
  CreateShortcut "$SMPROGRAMS\mistral-gui\mistral-gui.lnk" "$INSTDIR\mistral-gui.exe"
  CreateShortcut "$SMPROGRAMS\mistral-gui\Uninstall.lnk" "$INSTDIR\uninstall.exe"
  CreateShortcut "$DESKTOP\mistral-gui.lnk" "$INSTDIR\mistral-gui.exe"
  WriteRegStr HKCU "Software\mistral-gui" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "DisplayName" "mistral-gui"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui" "NoRepair" 1
SectionEnd
Function un.onInit
  !insertmacro MUI_UNGETLANGUAGE
FunctionEnd
Section "Uninstall"
  ExecWait '"$INSTDIR\mistral-gui.exe" --quit'
  Sleep 1000
  Delete "$INSTDIR\mistral-gui.exe"
  Delete "$INSTDIR\mistral-gui.exe.previous"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$DESKTOP\mistral-gui.lnk"
  Delete "$SMPROGRAMS\mistral-gui\mistral-gui.lnk"
  Delete "$SMPROGRAMS\mistral-gui\Uninstall.lnk"
  RMDir "$SMPROGRAMS\mistral-gui"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui"
  DeleteRegKey HKCU "Software\mistral-gui"
SectionEnd
