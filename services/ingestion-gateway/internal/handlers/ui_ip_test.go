package handlers

import (
	"net"
	"net/http"
	"testing"
)

func TestClientIPFromRequestSmoke(t *testing.T) {
	_, net1, _ := net.ParseCIDR("10.0.0.0/8")
	trustedProxyNets := []*net.IPNet{net1}

	header := http.Header{}
	header.Set("X-Forwarded-For", " 192.168.1.1 , 10.0.0.2")

	ip := clientIPFromRequestSmoke("10.0.0.1:1234", header, trustedProxyNets)
	if ip != "192.168.1.1" {
		t.Errorf("Expected 192.168.1.1, got %s", ip)
	}

	header.Set("X-Forwarded-For", "")
	header.Set("X-Real-IP", "192.168.1.2")
	ip = clientIPFromRequestSmoke("10.0.0.1", header, trustedProxyNets)
	if ip != "192.168.1.2" {
		t.Errorf("Expected 192.168.1.2, got %s", ip)
	}

	ip = clientIPFromRequestSmoke("192.168.1.3:1234", header, trustedProxyNets)
	if ip != "192.168.1.3" {
		t.Errorf("Expected 192.168.1.3, got %s", ip)
	}
}
