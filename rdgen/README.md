# rdgen: vendored reference (not a service)

Vendored copy of [bryangerlach/rdgen](https://github.com/bryangerlach/rdgen) (GPL-3.0).
DeskForge does not run it. It is kept as a reference for how a custom RustDesk client is
produced: the patches in `.github/patches/` (7 `.diff` files plus the `allowCustom.py`
and `privacyScreen.py` helpers) and the workflows in `.github/workflows/`, for example
`generator-windows.yml`. The three customization layers are described in
[PLAN.md](../PLAN.md#4-three-injection-layers-for-custom-client).

Do not copy these workflows into the RustDesk fork; the fork's own workflows are the only
executable source ([docs/FORKS-AND-PINS.md](../docs/FORKS-AND-PINS.md#1-topology)).

To run the original rdgen on its own, follow the
[upstream setup guide](https://github.com/bryangerlach/rdgen/blob/main/setup.md).
