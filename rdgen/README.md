# rdgen — vendored reference (not a service)

> DeskForge **does not run** rdgen as a service. This is a vendored copy of
> [bryangerlach/rdgen](https://github.com/bryangerlach/rdgen). Working instance:
> [rdgen.crayoneater.org](https://rdgen.crayoneater.org).

## Purpose

Reference implementation of the custom client generation workflow:
patches, `generator-windows.yml`, recipes for all 3 injection layers (config.rs / custom_.txt / branding).

## Patches (rdgen/.github/patches/)

`allowCustom`, `hidecm`, `removeSetupServerTip`, `removeNewVersionNotif`,
`cycle_monitor`, `xoffline`, `privacyScreen`.
