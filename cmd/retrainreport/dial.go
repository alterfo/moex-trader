package main

import (
	"context"
	"net"

	"golang.org/x/net/proxy"
)

type netConn = net.Conn

func dialVia(ctx context.Context, d proxy.Dialer, network, addr string) (net.Conn, error) {
	if cd, ok := d.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, network, addr)
	}
	return d.Dial(network, addr)
}
