package webfetch

import (
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	for _, test := range []struct {
		address string
		public  bool
	}{
		{"8.8.8.8", true},
		{"95.217.1.1", true},
		{"2606:4700:4700::1111", true},
		{"2a01:4f9:c010::1", true},

		{"127.0.0.1", false},
		{"127.1.2.3", false},
		{"::1", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"192.168.1.1", false},
		{"100.64.0.1", false},
		{"100.127.255.254", false},
		{"169.254.169.254", false},
		{"fe80::1", false},
		{"fc00::1", false},
		{"fd12:3456::1", false},
		{"224.0.0.1", false},
		{"239.255.255.250", false},
		{"ff02::1", false},
		{"ff05::2", false},
		{"0.0.0.0", false},
		{"0.1.2.3", false},
		{"::", false},
		{"255.255.255.255", false},
		{"240.0.0.1", false},
		{"192.0.0.8", false},
		{"192.0.2.1", false},
		{"198.18.0.1", false},
		{"2001:db8::1", false},
		{"fec0::1", false},

		// IPv4 addresses written as IPv6
		{"::ffff:127.0.0.1", false},
		{"::ffff:10.0.0.1", false},
		{"::ffff:169.254.169.254", false},
		{"::ffff:8.8.8.8", true},

		// IPv4 addresses behind NAT64
		{"64:ff9b::7f00:1", false},
		{"64:ff9b::a9fe:a9fe", false},
		{"64:ff9b::808:808", true},

		// Other IPv6 addresses that carry an IPv4 address
		{"::7f00:1", false},            // IPv4-compatible 127.0.0.1
		{"::808:808", false},           // IPv4-compatible, deprecated even for public addresses
		{"::ffff:0:7f00:1", false},     // IPv4-translated 127.0.0.1
		{"64:ff9b:1::a00:1", false},    // NAT64 inside a network
		{"2002:7f00:1::1", false},      // 6to4 of 127.0.0.1
		{"2002:a9fe:a9fe::1", false},   // 6to4 of 169.254.169.254
		{"2002:808:808::1", true},      // 6to4 of 8.8.8.8
		{"2001:0:4136:e378::1", false}, // Teredo
		{"100::1", false},              // discard-only
		{"2001:2::1", false},           // benchmarking
		{"2001:4860:4860::8888", true}, // an ordinary address just outside Teredo's prefix
	} {
		if got := isPublic(netip.MustParseAddr(test.address)); got != test.public {
			t.Errorf("isPublic(%s) = %v, want %v", test.address, got, test.public)
		}
	}
}

func TestGuardConnection(t *testing.T) {
	if err := guardConnection("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("a public address is refused: %v", err)
	}
	for _, address := range []string{"127.0.0.1:80", "[::1]:443", "[fe80::1%eth0]:80", "[::ffff:192.168.0.1]:8080"} {
		if err := guardConnection("tcp", address, nil); err == nil {
			t.Errorf("%s is allowed", address)
		}
	}
}
