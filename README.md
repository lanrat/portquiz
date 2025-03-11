

# portquiz

takes about 6min for all ports


## Server

The server creates a firewall rule to redirect all incomming traffic to the lisening IP to the port specified. If you use the same IP for remote access for services like SSH, IT WILL LOCK YOU OUT! This service needs a dedicated IP address.

```shell
# external IP on server
IP="192.0.2.123"
PORT="1337"

# start server
./portquiz-server -tcp -udp -listen "$IP:$PORT"
```

# TODO

* automatic firewall rule creation
* support testing variable packet/stream size to see if there is a bandwith or packet size cutoff

