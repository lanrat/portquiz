# portquiz

Test all outbound TCP/UDP ports for connectivity to a remote host.

Testing all ports can take 3-10 minutes for all ports depending on connection speed.

Can also be used to detect passive DPI Firewalls that block traffic not looking like the expected service for a given port.

## [Download Latest Release](https://github.com/lanrat/portquiz/releases)

## Server

The server creates a firewall rule to redirect all incoming traffic to the listening IP to the port specified. If you use the same IP for remote access for services like SSH, IT WILL LOCK YOU OUT! This service needs a dedicated IP address.

```shell
$ ./portquiz-server -h
Usage of ./portquiz-server:
  -listen string
        comma separated list of IPs to listen on (default "127.0.0.123")
  -no-iptables
        disable automatically creating iptables rules
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port uint
        default port to listen on which will have traffic redirected to (default 1337)
  -tcp
        start TCP server
  -timeout duration
        amount of time for each connection (default 10s)
  -udp
        start UDP server
  -verbose
        enable verbose logging
```

### Example Server

```shell
# start server
# listen on TCP and UDP ports
# listens on IPv4: 192.0.2.123 and IPv6: 2001:0DB8::1
./portquiz-server -tcp -udp -listen 192.0.2.123,2001:0DB8::1
```

## Client

The portquiz client connects to the portquiz server and tests port connectivity. By default portquiz will test all ports unless `-port` is specified.

IPv4 can be forced with `-4` and IPv6 can be forced with `-6`. If both are provided (`-4 -6`) then each port is tested using IPv4 and IPv6. If unspecified only one is tested.

```shell
$ ./portquiz-client -h
Usage of ./portquiz-client:
  -4    force IPv4
  -6    force IPv6
  -closed
        print only closed ports
  -multi uint
        test multiple times to ensure larger streams work (default 1)
  -open
        print only open ports
  -parallel uint
        number of worker threads (default 20)
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port string
        comma separated list of ports to test
  -retry uint
        retry count (default 3)
  -tcp
        start TCP client
  -timeout duration
        amount of time for each connection (default 5s)
  -udp
        start UDP client
  -verbose
        enable verbose logging
```

### Example Client

```shell
# test UDP and TCP ports, only print open ports
./portquiz-client -tcp -udp -open portquiz.example.com
```
