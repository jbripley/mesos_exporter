# Prometheus Mesos exporter

This is an exporter for Prometheus to monitor a Mesos cluster.

## Building and running

    make
    ./mesos_exporter <flags>

### Flags

Name                           | Description
-------------------------------|------------
web.listen-address             | Address to listen on for web interface and telemetry.
web.telemetry-path             | Path under which to expose metrics.
mesos.master-address           | Address to a Mesos master. The elected master will be auto-discovered.
log.level                      | Only log messages with the given severity or above. Valid levels: debug, info, warn, error, fatal, panic.


### Docker

A Docker container is available at
https://registry.hub.docker.com/u/prom/mesos-exporter

If you want to use it with your own configuration, you can mount it as a
volume:

    docker run -d -p 4000:4000 prom/mesos-exporter

It's also possible to use in your own Dockerfile:

    FROM prom/mesos-exporter

---
