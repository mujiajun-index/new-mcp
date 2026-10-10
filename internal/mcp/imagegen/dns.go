package imagegen

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type hostLookup func(context.Context, string) ([]net.IPAddr, error)

func lookupImageHost(ctx context.Context, host string, allowPrivate bool) ([]net.IPAddr, error) {
	return resolveImageHost(ctx, host, allowPrivate, net.DefaultResolver.LookupIPAddr, lookupPublicDNS)
}

// Fake-IP DNS used by TUN proxies does not reveal the destination address. Never
// dial these synthetic addresses: resolve the domain over authenticated HTTPS,
// then apply the same public-address checks and pin the resulting IP at dial time.
func resolveImageHost(ctx context.Context, host string, allowPrivate bool, system, fallback hostLookup) ([]net.IPAddr, error) {
	ips, err := system(ctx, host)
	if err != nil || allowPrivate || len(ips) == 0 {
		return ips, err
	}
	var realIPs []net.IPAddr
	hasFake := false
	for _, addr := range ips {
		if fakeIP(addr.IP) {
			hasFake = true
		} else {
			realIPs = append(realIPs, addr)
		}
	}
	if hasFake && net.ParseIP(host) == nil {
		ips = realIPs
		if len(ips) == 0 {
			ips, err = fallback(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("image host %q uses proxy Fake-IP DNS; real DNS lookup failed (exclude this domain from Fake-IP mode using fake-ip-filter, or configure real DNS): %w", host, err)
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("image host %q has no addresses", host)
	}
	for _, addr := range ips {
		if !publicIP(addr.IP) {
			return nil, fmt.Errorf("image host %q resolves to prohibited network address %s", host, addr.IP)
		}
	}
	return ips, nil
}

func fakeIP(ip net.IP) bool {
	ip = ip.To4()
	return ip != nil && ip[0] == 198 && (ip[1] == 18 || ip[1] == 19)
}

// Only domains whose system DNS returns the synthetic 198.18.0.0/15 range are
// sent to Cloudflare's public DoH resolver. Its bootstrap IPs bypass system DNS;
// TLS still verifies cloudflare-dns.com, and no model credentials are attached.
func lookupPublicDNS(ctx context.Context, host string) ([]net.IPAddr, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	transport := &http.Transport{TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 4 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "cloudflare-dns.com:443" {
			return nil, fmt.Errorf("unexpected DNS resolver address")
		}
		dialer := net.Dialer{Timeout: 2 * time.Second}
		var err error
		for _, ip := range []string{"1.1.1.1:443", "1.0.0.1:443"} {
			var conn net.Conn
			conn, err = dialer.DialContext(ctx, network, ip)
			if err == nil {
				return conn, nil
			}
		}
		return nil, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return queryPublicDNS(ctx, client, "https://cloudflare-dns.com/dns-query", host)
}

func queryPublicDNS(ctx context.Context, client *http.Client, endpoint, host string) ([]net.IPAddr, error) {
	type result struct {
		ips []net.IPAddr
		err error
	}
	results := make(chan result, 2)
	for _, recordType := range []int{1, 28} {
		go func() {
			ips, err := queryDNSRecord(ctx, client, endpoint, host, recordType)
			results <- result{ips, err}
		}()
	}
	var ips []net.IPAddr
	var lastErr error
	for range 2 {
		r := <-results
		ips = append(ips, r.ips...)
		if r.err != nil {
			lastErr = r.err
		}
	}
	if len(ips) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("public DNS returned no addresses")
	}
	return ips, nil
}

func queryDNSRecord(ctx context.Context, client *http.Client, endpoint, host string, recordType int) ([]net.IPAddr, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("name", host)
	query.Set("type", fmt.Sprint(recordType))
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public DNS HTTP %d", resp.StatusCode)
	}
	data, err := readBounded(resp.Body, 64<<10)
	if err != nil {
		return nil, fmt.Errorf("public DNS response: %w", err)
	}
	var answer struct {
		Status *int
		TC     bool
		Answer []struct {
			Type int
			Data string
		}
	}
	if err = json.Unmarshal(data, &answer); err != nil || answer.Status == nil {
		return nil, fmt.Errorf("invalid public DNS response")
	}
	if answer.TC || (*answer.Status != 0 && *answer.Status != 3) {
		return nil, fmt.Errorf("public DNS query failed (status %d)", *answer.Status)
	}
	var ips []net.IPAddr
	for _, record := range answer.Answer {
		if *answer.Status != 0 || record.Type != recordType {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(record.Data))
		if ip == nil || (recordType == 1 && ip.To4() == nil) || (recordType == 28 && ip.To4() != nil) {
			return nil, fmt.Errorf("invalid public DNS address")
		}
		ips = append(ips, net.IPAddr{IP: ip})
	}
	return ips, nil
}
