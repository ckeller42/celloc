# Glossary

```{glossary}
:sorted:

AP
BSSID
  WiFi access point and its MAC address, as seen in `iw` scan output.

MCC
MNC
  Mobile country code and mobile network code of the serving cell.

CID
TAC
  Cell identity and tracking area code (hex in the AT reply, decimal in celloc).

LTE
NR5G-NSA
NR5G-SA
  Radio technologies in `AT+QENG` lines. NSA is 5G anchored on an LTE cell.

AT+QENG
  Quectel modem command that reports the serving cell.

gpsd
  Daemon and JSON protocol that location clients speak. celloc implements a subset.

TPV
SKY
  gpsd time-position-velocity and satellite reports.

eph
epx
epy
  Estimated horizontal error radius, and per-axis error, in metres.

wifix
cellfix
  celloc's non-standard TPV objects that say what resolved the fix.

uci
  OpenWrt's configuration system.

procd
  OpenWrt's service supervisor.

opkg
ipk
  OpenWrt's package manager and its package format.

Fix
  `source.Fix`, celloc's internal position estimate.
```
