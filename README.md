

# portquiz

takes about 6min for all ports


## Server

```shell
# external IP on server
IP="192.0.2.123"
PORT="1337"

# create IP tables rules to map all TCP & UDP ports to single port for the given IP
iptables -t nat -A PREROUTING --destination "$IP" -p tcp -j DNAT --to-destination ":$PORT"
iptables -t nat -A PREROUTING --destination "$IP" -p udp -j DNAT --to-destination ":$PORT"

# start server
./portquiz-server -tcp -udp -listen "$IP:$PORT"
```

# TODO

* automatic firewall rule creation
* support testing variable packet/stream size to see if there is a bandwith or packet size cutoff

