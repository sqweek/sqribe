#include "sqweek.mmh"

#define UISAMPLE_BLINE_TEXT	sqribe by sqweek
#(
	#define UISAMPLE_LEFTSIDE_TEXT
	Sqribe
	by sqweek
#)
#(
	#define UISAMPLE_WELCOME_VB_EXPRESSION_FIRST_PARA
	"This will install [ProductName] v[ProductVersion]
	onto your computer."
#)
#define+ DEFAULT_COMPONENT_ATTRIBUTES 64bit

; TODO branding - icon, logo?  UISAMPLE_BITMAP_BANNER_GRAPHIC


; TODO manifest for high DPI?
; TODO additional GUID for "exe-only" update product?

;#DefineRexx ''
;call SetEnv 'MAKEMSI_MM_ROOTDIR.P', 'dist\*.*';
;#DefineRexx


<$DirectoryTree Dir="[ProgramFiles64Folder]\sqribe" Key="INSTALLDIR" CHANGE="\" PrimaryFolder="Y">

<$Component "Binaries" Create="Y" Directory_="INSTALLDIR">
	<$File Source="sqribe.exe">
	<$File Source="avcodec-56.dll">
	<$File Source="avformat-56.dll">
	<$File Source="avutil-54.dll">
	<$File Source="libfluidsynth.dll">
	<$File Source="libglib-2.0-0.dll">
	<$File Source="libgthread-2.0-0.dll">
	<$File Source="libiconv-2.dll">
	<$File Source="libintl-8.dll">
	<$File Source="libportaudio-2.dll">
	<$File Source="libwinpthread-1.dll">   ;; XXX what's license on this?
	<$File Source="swresample-1.dll">
	<$File Source="LICENSE">
	<$File Source="README.md" Destination="README.txt">
<$/Component>

<$Component "Font" Create="Y" Directory_="INSTALLDIR">
	<$File Source="luxisr.ttf">
<$/Component>

<$Component "SoundFont" Create="Y" Directory_="INSTALLDIR">
	<$File Source="C:\Users\sqwee\Desktop\synth\FluidR3_GM.sf2">
<$/Component>

<$Component "SQS_Association" Create="Y" Directory_="INSTALLDIR" CU="?">
    <$Extn ".sqs" HKEY="HKMU" Description="Sqribe save" Icon=^"[INSTALLDIR]sqribe.exe",0^>
		<$ExtnAction Key="Open" Description="Open" Command=^"[INSTALLDIR]sqribe.exe" "%1"^>
	<$/Extn>
<$/Component>

<$Component "System_Associations" Create="Y" Directory_="INSTALLDIR" CU="?">
	<$SystemFileAssociation Type="audio" Name="Transqribe" Desc="Trans&qribe" Command=^"[INSTALLDIR]sqribe.exe" "%1"^>
	<$SystemFileAssociation Type="video" Name="Transqribe" Desc="Trans&qribe" Command=^"[INSTALLDIR]sqribe.exe" "%1"^>
<$/Component>

<$Component "Shortcut" Create="Y" Directory_="INSTALLDIR">
	#(
		<$Shortcut
			Dir="[ProgramMenuFolder]"
			Target="[INSTALLDIR]sqribe.exe"
			Title="Sqribe"
			Description="Sqribe by sqweek"
			WorkDir="INSTALLDIR"
		>
	#)
<$/Component>

;;<$Component "Audio_Association" Create="Y" Directory_="INSTALLDIR" CU="?">
;;	<$Registry HKEY="HKMU" Key="SOFTWARE\Classes\SystemFileAssociations\audio\shell\Transqribe" KeyAction="INSTALL_UNINSTALL">
;;	<$Registry HKEY="HKMU" Value="Trans&qribe" Key="SOFTWARE\Classes\SystemFileAssociations\audio\shell\Transqribe">
;;	<$Registry HKEY="HKMU" Type="EXPSTRING" Value=^"[INSTALLDIR]sqribe.exe" "%1"^ Key="SOFTWARE\Classes\SystemFileAssociations\audio\shell\Transqribe\command" MsiFormatted="VALUE">
;;<$/Component>



; HKEY_CLASSES_ROOT\Stack.Audio