package application_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

func TestDestinationPolicyAddresses(t *testing.T) {
	cases := []struct {
		addr       string
		production bool
		dev        bool
	}{
		{"93.184.215.14", true, true},
		{"2606:4700::6810:84e5", true, true},
		{"127.0.0.1", false, true},
		{"127.255.0.9", false, true},
		{"::1", false, true},
		{"10.1.2.3", false, true},
		{"172.16.0.1", false, true},
		{"172.32.0.1", true, true},
		{"192.168.1.1", false, true},
		{"fd00:ec2::254", false, true},
		{"0.0.0.0", false, false},
		{"::", false, false},
		{"169.254.169.254", false, false},
		{"fe80::1", false, false},
		{"100.100.100.200", false, false},
		{"192.0.2.10", false, false},
		{"198.18.0.1", false, false},
		{"224.0.0.1", false, false},
		{"255.255.255.255", false, false},
		{"2001:db8::1", false, false},
		{"::ffff:127.0.0.1", false, true},
		{"::ffff:169.254.169.254", false, false},
		{"64:ff9b::a9fe:a9fe", false, false}, // NAT64 of 169.254.169.254
		{"64:ff9b::5db8:d70e", true, true},   // NAT64 of 93.184.215.14
		{"2002:7f00:1::1", false, true},      // 6to4 of 127.0.0.1
		{"2002:a9fe:a9fe::1", false, false},  // 6to4 of 169.254.169.254
	}
	production := application.DestinationPolicy{}
	dev := application.DestinationPolicy{AllowPrivateNetworks: true}
	for _, tc := range cases {
		addr := netip.MustParseAddr(tc.addr)
		if got := production.AllowAddr(addr); got != tc.production {
			t.Errorf("production %s = %v, want %v", tc.addr, got, tc.production)
		}
		if got := dev.AllowAddr(addr); got != tc.dev {
			t.Errorf("development %s = %v, want %v", tc.addr, got, tc.dev)
		}
	}
	if production.AllowAddr(netip.Addr{}) {
		t.Error("zero address allowed")
	}
}

func TestEndpointURLsRespectDestinationPolicy(t *testing.T) {
	ctx := context.Background()
	prod := newFixture(t, false)
	for _, raw := range []string{
		"https://127.0.0.1/hook",
		"https://[::1]:8443/hook",
		"https://localhost/hook",
		"https://api.localhost/hook",
		"https://LOCALHOST./hook",
		"https://169.254.169.254/latest/meta-data/",
		"https://10.0.0.5/hook",
		"https://[::ffff:192.168.0.1]/hook",
	} {
		_, _, err := prod.endpoints.Create(ctx, prod.principal, prod.project.ID, application.CreateEndpointInput{URL: raw})
		if !errors.Is(err, application.ErrDestinationNotAllowed) {
			t.Errorf("production %s: err = %v", raw, err)
		}
	}
	if _, _, err := prod.endpoints.Create(ctx, prod.principal, prod.project.ID, application.CreateEndpointInput{URL: "https://hooks.example.com/in"}); err != nil {
		t.Fatalf("public hostname: %v", err)
	}

	dev := newFixture(t, true)
	if _, _, err := dev.endpoints.Create(ctx, dev.principal, dev.project.ID, application.CreateEndpointInput{URL: "http://127.0.0.1:9000/hook"}); err != nil {
		t.Fatalf("development loopback: %v", err)
	}
	_, _, err := dev.endpoints.Create(ctx, dev.principal, dev.project.ID, application.CreateEndpointInput{URL: "http://169.254.169.254/"})
	if !errors.Is(err, application.ErrDestinationNotAllowed) {
		t.Fatalf("development metadata: err = %v", err)
	}
}

func TestEndpointUpdateRespectsDestinationPolicy(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, false)
	endpoint, _, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://hooks.example.com/in"})
	if err != nil {
		t.Fatal(err)
	}
	blocked := "https://169.254.169.254/"
	if _, err := f.endpoints.Update(ctx, f.principal, endpoint.ID, domain.EndpointUpdate{URL: &blocked}); !errors.Is(err, application.ErrDestinationNotAllowed) {
		t.Fatalf("update: err = %v", err)
	}
}
