# portquiz

Test all outbound TCP/UDP ports for connectivity to a remote host.

Testing all ports can take 3-10 minutes for all ports dependig on connection speed.

Can also be used to detect passive DPI Firewalls that block traffic not looking like the expected service for a given port.

## Server

The server creates a firewall rule to redirect all incoming traffic to the listening IP to the port specified. If you use the same IP for remote access for services like SSH, IT WILL LOCK YOU OUT! This service needs a dedicated IP address.

```shell
# external IP on server
IP="192.0.2.123"

# start server
./portquiz-server -tcp -udp -listen "$IP"
```

## Client

```shell
./portquiz-client -tcp -udp -open "$IP"
```
