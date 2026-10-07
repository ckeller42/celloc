celloc
======

celloc gives a router without a GPS antenna a position. ``geolocd`` scans nearby WiFi access points
and reads the modem's serving cell, lets a geolocation provider resolve both together, and serves
the result on a gpsd socket, so any gpsd client can use it. ``geoinflux`` uploads the fixes to
InfluxDB. A WiFi or cell fix is never presented as a GPS fix.

.. toctree::
   :caption: Getting started
   :maxdepth: 1

   getting-started

.. toctree::
   :caption: How-to guides
   :maxdepth: 1

   INSTALL

.. toctree::
   :caption: Reference
   :maxdepth: 1

   reference/gpsd
   reference/config
   reference/influxdb

.. toctree::
   :caption: Explanation
   :maxdepth: 1

   ARCHITECTURE
