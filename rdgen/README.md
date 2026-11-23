# rdgen — vendored reference (not a service)

> DeskForge **does not run** rdgen as a service. This is a vendored copy of
> [bryangerlach/rdgen](https://github.com/bryangerlach/rdgen).

## Purpose

Reference implementation of the custom client generation workflow:
patches, `generator-windows.yml`, recipes for all 3 injection layers (config.rs / custom_.txt / branding).

## Patches (rdgen/.github/patches/)

`allowCustom`, `cycle_monitor`, `flutter_3.24.4_dropdown_menu_enableFilter`, `hidecm`, `removeNewVersionNotif`, `removeSetupServerTip`, `xoffline` (`.diff`), plus the `allowCustom.py` / `privacyScreen.py` helpers.
