# rdgen — vendored reference (не сервис)

> DeskForge **не запускает** rdgen как сервис. Код — vendored копия
> [bryangerlach/rdgen](https://github.com/bryangerlach/rdgen). Работающий аналог —
> [rdgen.crayoneater.org](https://rdgen.crayoneater.org).

## Для чего

Это reference implementation workflow генерации кастомного клиента:
патчи, `generator-windows.yml`, рецепты 3 слоёв инъекции (config.rs / custom_.txt / branding).

## Патчи (rdgen/.github/patches/)

`allowCustom`, `hidecm`, `removeSetupServerTip`, `removeNewVersionNotif`,
`cycle_monitor`, `xoffline`, `privacyScreen`.
