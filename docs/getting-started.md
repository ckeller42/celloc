# Getting started

This walk-through takes you from nothing to a position fix in InfluxDB. It uses the defaults:
Google as provider, a GL-iNet router with a Quectel modem, and a Pi on the same LAN. Each step links
to the how-to with the variants.

You need a Google Geolocation API key (see [Install and operate](INSTALL.md#configure-wifi-geolocation))
and ssh access to the router.

## 1. Install `geolocd` on the router

```sh
echo 'src/gz celloc https://ckeller42.github.io/celloc/aarch64_cortex-a53' >> /etc/opkg/customfeeds.conf
opkg update; opkg install geolocd
```

The package installs the binary, a procd service and a default `/etc/config/geolocd`.

## 2. Set the key and restart

```sh
uci set geolocd.main.google_key='AIza...your_google_geolocation_key'
uci commit geolocd && /etc/init.d/geolocd restart
```

## 3. Check the position

```sh
gpspipe -w <router-ip>:2947
```

Expect a `TPV` with `mode` 2, `lat`/`lon`, a `wifix` object and a small `eph`. A router that
resolves from the serving cell only reports `cellfix` and an `eph` of hundreds of metres or more.
The fields are explained in [gpsd output](reference/gpsd.md).

## 4. Upload to InfluxDB (optional)

On the Pi, install `geoinflux`, put the InfluxDB token and addresses in `/etc/buspi/geo.env`, and
start the service. Steps: [Install and operate](INSTALL.md#run-the-uploader-geoinflux-on-the-pi).
The points you get are described in the [InfluxDB schema](reference/influxdb.md).

## Where next

- Why a WiFi fix is `mode=2` and never GPS: [Architecture](ARCHITECTURE.md#8-crosscutting-concepts).
- Every uci option and flag: [Configuration](reference/config.md).
