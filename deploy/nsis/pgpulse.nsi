; PGPulse — Windows Installer (NSIS)
; Build: makensis deploy/nsis/pgpulse.nsi
; Requires: pgpulse-desktop.exe built with -tags desktop -ldflags="-s -w -H windowsgui"
;
; Expected file layout when running makensis:
;   deploy/nsis/pgpulse.nsi          (this script)
;   deploy/nsis/license.txt          (MIT license)
;   deploy/nsis/configs/pgpulse.example.yml
;   deploy/nsis/pgpulse-desktop.exe  (built desktop binary)
;
; Output: pgpulse-setup.exe (in the directory where makensis is invoked)

;---------------------------------------------------------------------------
; Build settings
;---------------------------------------------------------------------------
!include "MUI2.nsh"

Unicode True

Name "PGPulse"
OutFile "pgpulse-setup.exe"
InstallDir "$PROGRAMFILES\PGPulse"
InstallDirRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" "InstallLocation"
RequestExecutionLevel admin

;---------------------------------------------------------------------------
; Version info embedded in the setup exe
;---------------------------------------------------------------------------
!define PRODUCT_NAME    "PGPulse"
!define PRODUCT_VERSION "1.0.0"
!define PRODUCT_PUBLISHER "PGPulse Project"

VIProductVersion "1.0.0.0"
VIAddVersionKey "ProductName"     "${PRODUCT_NAME}"
VIAddVersionKey "ProductVersion"  "${PRODUCT_VERSION}"
VIAddVersionKey "CompanyName"     "${PRODUCT_PUBLISHER}"
VIAddVersionKey "FileDescription" "PGPulse Installer"
VIAddVersionKey "FileVersion"     "${PRODUCT_VERSION}"
VIAddVersionKey "LegalCopyright"  "Copyright (c) 2026 ${PRODUCT_PUBLISHER}"

;---------------------------------------------------------------------------
; MUI2 pages
;---------------------------------------------------------------------------
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "license.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; Uninstaller pages
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; Language
!insertmacro MUI_LANGUAGE "English"

;---------------------------------------------------------------------------
; Installer sections
;---------------------------------------------------------------------------
Section "PGPulse (required)" SecCore
    SectionIn RO

    ; Install main executable
    SetOutPath "$INSTDIR"
    SetOverwrite on
    File "/oname=pgpulse.exe" "pgpulse-desktop.exe"

    ; Install example config — do NOT overwrite existing config on upgrade
    SetOverwrite off
    File "/oname=pgpulse.yml" "configs\pgpulse.example.yml"
    SetOverwrite on

    ; Create uninstaller
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; Start Menu shortcuts
    CreateDirectory "$SMPROGRAMS\PGPulse"
    CreateShortCut "$SMPROGRAMS\PGPulse\PGPulse.lnk" "$INSTDIR\pgpulse.exe" \
        "--mode=desktop" "$INSTDIR\pgpulse.exe" 0
    CreateShortCut "$SMPROGRAMS\PGPulse\Uninstall PGPulse.lnk" "$INSTDIR\uninstall.exe"

    ; Desktop shortcut
    CreateShortCut "$DESKTOP\PGPulse.lnk" "$INSTDIR\pgpulse.exe" \
        "--mode=desktop" "$INSTDIR\pgpulse.exe" 0

    ; Add/Remove Programs registry keys
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "DisplayName" "${PRODUCT_NAME}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "DisplayVersion" "${PRODUCT_VERSION}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "Publisher" "${PRODUCT_PUBLISHER}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "InstallLocation" "$INSTDIR"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "DisplayIcon" "$INSTDIR\pgpulse.exe"
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "EstimatedSize" 20480
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "NoModify" 1
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse" \
        "NoRepair" 1
SectionEnd

Section "Start with Windows" SecAutostart
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" \
        "PGPulse" '"$INSTDIR\pgpulse.exe" --mode=desktop'
SectionEnd

;---------------------------------------------------------------------------
; Section descriptions
;---------------------------------------------------------------------------
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} \
        "Install PGPulse PostgreSQL monitor (required)."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecAutostart} \
        "Start PGPulse automatically when you log in to Windows."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

;---------------------------------------------------------------------------
; Uninstaller
;---------------------------------------------------------------------------
Section "Uninstall"
    ; Remove files
    Delete "$INSTDIR\pgpulse.exe"
    Delete "$INSTDIR\pgpulse.yml"
    Delete "$INSTDIR\uninstall.exe"

    ; Remove shortcuts
    Delete "$DESKTOP\PGPulse.lnk"
    Delete "$SMPROGRAMS\PGPulse\PGPulse.lnk"
    Delete "$SMPROGRAMS\PGPulse\Uninstall PGPulse.lnk"
    RMDir "$SMPROGRAMS\PGPulse"

    ; Remove registry keys
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\PGPulse"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "PGPulse"

    ; Remove install directory (only if empty)
    RMDir "$INSTDIR"
SectionEnd
