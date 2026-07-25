Unicode true

####
## Open Monitoring installer - a deliberately modern, minimal flow:
##
##   [options: logo, PawnIO checkbox, install path] -> [progress] -> [done]
##
## No MUI: the classic wizard chrome (welcome page, white header band,
## sidebar bitmaps) is what makes installers look twenty years old. Instead
## the raw pages are styled dark (app palette) via SetCtlColors, and the
## title bar is switched to dark mode through DWM, matching Windows 11.
##
## Wails populates the INFO_* defines through wails_tools.nsh; see the
## original template notes in the Wails docs for building this manually.
####
!include "wails_tools.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

Name "${INFO_PRODUCTNAME}"
Caption "${INFO_PRODUCTNAME} Setup"
BrandingText " "
Icon "..\icon.ico"
UninstallIcon "..\icon.ico"
XPStyle on
ShowInstDetails hide
ShowUninstDetails hide

# The app's dark palette (app.css); SetCtlColors wants 0xRRGGBB.
!define COL_TEXT 0xEEF1F4
!define COL_MUT  0x9AA3AD
!define COL_DIM  0x5F6873
!define COL_BG   0x0E1013
!define COL_CARD 0x15181D

# Checkboxes are themed BUTTON controls, and themed rendering ignores the
# colors SetCtlColors sets - the label would stay black on the dark page.
# Stripping the visual style from that one control makes it classic-drawn,
# where the colors apply.
!macro DARK_CHECKBOX hwnd
   System::Call 'uxtheme::SetWindowTheme(p ${hwnd}, w " ", w " ")'
   SetCtlColors ${hwnd} ${COL_TEXT} ${COL_BG}
!macroend

# Buttons cannot be recolored, but Windows 10 1809+ ships a dark theme class
# the system's own dark dialogs use. Re-theming a button as DarkMode_Explorer
# renders it dark grey with proper hover - matching the app's card buttons.
# On older systems SetWindowTheme just fails and the button stays default.
!macro DARK_BUTTON hwnd
   System::Call 'uxtheme::SetWindowTheme(p ${hwnd}, w "DarkMode_Explorer", p 0)'
!macroend

# DarkMode_CFD is the class the common file dialog uses for dark edit fields.
!macro DARK_EDIT hwnd
   System::Call 'uxtheme::SetWindowTheme(p ${hwnd}, w "DarkMode_CFD", p 0)'
   SetCtlColors ${hwnd} ${COL_TEXT} ${COL_CARD}
!macroend

OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif

Var InstallPawnIO # 1 when the PawnIO checkbox is (or stays) checked
Var PawnIOCheck
Var DirField
Var LaunchCheck
Var LogoHandle
Var TitleFont
Var SubFont

Page custom OptionsCreate OptionsLeave
Page instfiles "" InstFilesShow
Page custom FinishCreate FinishLeave

UninstPage instfiles "" un.InstFilesShow

Function .onInit
   !insertmacro wails.checkArchitecture
   StrCpy $InstallPawnIO 1 # default on, also for silent (/S) installs
   InitPluginsDir
   File "/oname=$PLUGINSDIR\logo.bmp" "logo.bmp"
FunctionEnd

Function un.onInit
FunctionEnd

# Dark window chrome, shared by installer and uninstaller: dark title bar
# (DWMWA_USE_IMMERSIVE_DARK_MODE = 20; silently ignored before Win10 20H1),
# dark dialog background, and no divider line above the buttons.
!macro DARK_CHROME
   System::Call 'dwmapi::DwmSetWindowAttribute(p $HWNDPARENT, i 20, *i 1, i 4)'
   # SetPreferredAppMode(ForceDark): lets DarkMode_* theme classes take effect
   # process-wide. Undocumented but stable since Win10 1809 (ordinal 135).
   System::Call 'uxtheme::#135(i 2)'
   SetCtlColors $HWNDPARENT ${COL_TEXT} ${COL_BG}
   GetDlgItem $0 $HWNDPARENT 1028 # branding text
   SetCtlColors $0 ${COL_DIM} ${COL_BG}
   GetDlgItem $0 $HWNDPARENT 1256 # branding text (right half)
   SetCtlColors $0 ${COL_DIM} ${COL_BG}
   GetDlgItem $0 $HWNDPARENT 1035 # divider line above the buttons
   ShowWindow $0 ${SW_HIDE}
   # The wizard's own bottom buttons (Next / Cancel / Back)
   GetDlgItem $0 $HWNDPARENT 1
   !insertmacro DARK_BUTTON $0
   GetDlgItem $0 $HWNDPARENT 2
   !insertmacro DARK_BUTTON $0
   GetDlgItem $0 $HWNDPARENT 3
   !insertmacro DARK_BUTTON $0
!macroend

Function .onGUIInit
   !insertmacro DARK_CHROME
FunctionEnd

Function un.onGUIInit
   !insertmacro DARK_CHROME
FunctionEnd

# ---- Page 1: everything the user decides, on one dark page ----------------

Function OptionsCreate
   nsDialogs::Create 1018
   Pop $9
   ${If} $9 == error
      Abort
   ${EndIf}
   SetCtlColors $9 ${COL_TEXT} ${COL_BG}

   ${NSD_CreateBitmap} 0u 6u 40u 40u ""
   Pop $0
   ${NSD_SetImage} $0 "$PLUGINSDIR\logo.bmp" $LogoHandle

   CreateFont $TitleFont "Segoe UI" "16" "600"
   CreateFont $SubFont "Segoe UI" "9" "400"

   ${NSD_CreateLabel} 46u 8u 210u 18u "${INFO_PRODUCTNAME}"
   Pop $0
   SetCtlColors $0 ${COL_TEXT} ${COL_BG}
   SendMessage $0 ${WM_SETFONT} $TitleFont 1

   ${NSD_CreateLabel} 47u 27u 210u 12u "v${INFO_PRODUCTVERSION} - PC monitoring, HUD and FPS overlay"
   Pop $0
   SetCtlColors $0 ${COL_MUT} ${COL_BG}
   SendMessage $0 ${WM_SETFONT} $SubFont 1

   ${NSD_CreateCheckbox} 2u 58u 260u 12u "Install the PawnIO driver (required for CPU temperature && power)"
   Pop $PawnIOCheck
   !insertmacro DARK_CHECKBOX $PawnIOCheck
   ${NSD_Check} $PawnIOCheck

   ${NSD_CreateLabel} 13u 72u 250u 20u "Open-source kernel driver (pawnio.eu), installed system-wide through winget. Without it Open Monitoring stays on its install screen."
   Pop $0
   SetCtlColors $0 ${COL_MUT} ${COL_BG}

   ${NSD_CreateLabel} 2u 102u 60u 12u "Install to:"
   Pop $0
   SetCtlColors $0 ${COL_MUT} ${COL_BG}

   ${NSD_CreateText} 2u 114u 198u 13u "$INSTDIR"
   Pop $DirField
   !insertmacro DARK_EDIT $DirField

   ${NSD_CreateButton} 206u 113u 54u 15u "Browse..."
   Pop $0
   !insertmacro DARK_BUTTON $0
   ${NSD_OnClick} $0 OptionsBrowse

   # The primary action is installing, so the button says so; there is no
   # page to go back to and nothing else to configure.
   GetDlgItem $0 $HWNDPARENT 1
   SendMessage $0 ${WM_SETTEXT} 0 "STR:Install"
   GetDlgItem $0 $HWNDPARENT 3
   ShowWindow $0 ${SW_HIDE}

   nsDialogs::Show
FunctionEnd

Function OptionsBrowse
   ${NSD_GetText} $DirField $0
   nsDialogs::SelectFolderDialog "Choose the install folder" "$0"
   Pop $0
   ${If} $0 != error
      ${NSD_SetText} $DirField "$0"
   ${EndIf}
FunctionEnd

Function OptionsLeave
   ${NSD_GetState} $PawnIOCheck $InstallPawnIO
   ${NSD_GetText} $DirField $0
   ${If} $0 != ""
      StrCpy $INSTDIR $0
   ${EndIf}
FunctionEnd

# ---- Page 2: progress ------------------------------------------------------

!macro DARK_INSTFILES
   FindWindow $0 "#32770" "" $HWNDPARENT
   SetCtlColors $0 ${COL_TEXT} ${COL_BG}
   GetDlgItem $1 $0 1006 # status line
   SetCtlColors $1 ${COL_MUT} ${COL_BG}
   GetDlgItem $1 $0 1016 # details list, revealed by the details button
   SetCtlColors $1 ${COL_MUT} ${COL_CARD}
   !insertmacro DARK_BUTTON $1 # dark scrollbar for the list
   GetDlgItem $1 $0 1027 # "Show details" button
   !insertmacro DARK_BUTTON $1
!macroend

Function InstFilesShow
   !insertmacro DARK_INSTFILES
FunctionEnd

Function un.InstFilesShow
   !insertmacro DARK_INSTFILES
FunctionEnd

# ---- Page 3: done ----------------------------------------------------------

Function FinishCreate
   nsDialogs::Create 1018
   Pop $9
   SetCtlColors $9 ${COL_TEXT} ${COL_BG}

   ${NSD_CreateBitmap} 0u 6u 40u 40u ""
   Pop $0
   ${NSD_SetImage} $0 "$PLUGINSDIR\logo.bmp" $LogoHandle

   ${NSD_CreateLabel} 46u 8u 210u 18u "All set"
   Pop $0
   SetCtlColors $0 ${COL_TEXT} ${COL_BG}
   SendMessage $0 ${WM_SETFONT} $TitleFont 1

   ${NSD_CreateLabel} 47u 27u 210u 12u "${INFO_PRODUCTNAME} ${INFO_PRODUCTVERSION} is installed."
   Pop $0
   SetCtlColors $0 ${COL_MUT} ${COL_BG}
   SendMessage $0 ${WM_SETFONT} $SubFont 1

   ${NSD_CreateCheckbox} 2u 58u 200u 12u "Launch ${INFO_PRODUCTNAME}"
   Pop $LaunchCheck
   !insertmacro DARK_CHECKBOX $LaunchCheck
   ${NSD_Check} $LaunchCheck

   GetDlgItem $0 $HWNDPARENT 1
   SendMessage $0 ${WM_SETTEXT} 0 "STR:Finish"
   GetDlgItem $0 $HWNDPARENT 2 # Cancel makes no sense once installed
   EnableWindow $0 0
   GetDlgItem $0 $HWNDPARENT 3
   ShowWindow $0 ${SW_HIDE}

   nsDialogs::Show
FunctionEnd

Function FinishLeave
   ${NSD_GetState} $LaunchCheck $0
   ${If} $0 == 1
      Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
   ${EndIf}
FunctionEnd

# ---- Install ---------------------------------------------------------------

Section "-install"
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller

    # PawnIO is the kernel driver the app reads CPU package temperature and
    # power through; the app refuses to run without it. Installed through
    # winget from the official namazso.PawnIO package - the installer never
    # downloads binaries itself. A failure is reported but does not abort,
    # because the app offers the same install again at first launch.
    ${If} $InstallPawnIO == 1
        DetailPrint "Installing the PawnIO driver (winget, package namazso.PawnIO)..."
        nsExec::ExecToLog 'winget install --id namazso.PawnIO --exact --silent --accept-package-agreements --accept-source-agreements'
        Pop $0
        ${If} $0 != 0
            DetailPrint "PawnIO could not be installed automatically (exit code $0)."
            DetailPrint "It may already be present, or winget is unavailable - the app will offer the install again at first launch."
        ${EndIf}
    ${EndIf}
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller

    # PawnIO stays: it is a system-wide driver other tools may rely on.
SectionEnd
