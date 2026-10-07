package route

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestGateTruncatesUDPAndPreservesTCP(t *testing.T) {
	upUDP, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upUDP.Close()
	upTCP, err := net.Listen("tcp", upUDP.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer upTCP.Close()
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		r := new(dns.Msg)
		r.SetReply(q)
		if _, ok := w.RemoteAddr().(*net.UDPAddr); ok {
			r.Truncated = true
		} else {
			for i := 0; i < 12; i++ {
				r.Answer = append(r.Answer, &dns.TXT{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60}, Txt: []string{strings.Repeat("x", 150)}})
			}
		}
		_ = w.WriteMsg(r)
	})
	for _, server := range []*dns.Server{{PacketConn: upUDP, Handler: handler}, {Listener: upTCP, Handler: handler}} {
		started := make(chan struct{})
		server.NotifyStartedFunc = func() { close(started) }
		go server.ActivateAndServe()
		<-started
		defer server.Shutdown()
	}
	c := testConfig()
	c.Upstream = upUDP.LocalAddr().String()
	c.DNSDeadlineMS = 3000
	a, err := New(context.Background(), c, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	gatePacket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gateTCP, err := net.Listen("tcp", gatePacket.LocalAddr().String())
	if err != nil {
		gatePacket.Close()
		t.Fatal(err)
	}
	for _, gate := range []*dns.Server{{PacketConn: gatePacket, Handler: a}, {Listener: gateTCP, Handler: a}} {
		started := make(chan struct{})
		gate.NotifyStartedFunc = func() { close(started) }
		go gate.ActivateAndServe()
		<-started
		defer gate.Shutdown()
	}
	for _, limit := range []uint16{0, 128, 1232, 4096} {
		t.Run(fmt.Sprintf("udp_limit_%d", limit), func(t *testing.T) {
			q := new(dns.Msg)
			q.SetQuestion("large.route-lab.test.", dns.TypeTXT)
			if limit != 0 {
				q.SetEdns0(limit, false)
			}
			budget := max(limit, dns.MinMsgSize)
			wire, _ := q.Pack()
			conn, err := net.Dial("udp", gatePacket.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			if _, err = conn.Write(wire); err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, 4096)
			n, err := conn.Read(buffer)
			if err != nil {
				t.Fatal(err)
			}
			response := new(dns.Msg)
			if err := response.Unpack(buffer[:n]); err != nil {
				t.Fatal(err)
			}
			if n > int(budget) {
				t.Errorf("Gate exceeded downstream UDP limit: %d > %d", n, budget)
			}
			if response.Truncated != (limit != 4096) {
				t.Errorf("unexpected TC flag: %v for size %d", response.Truncated, budget)
			}
			client := &dns.Client{Net: "udp", UDPSize: budget, Timeout: time.Second}
			limited, _, clientErr := client.Exchange(q, gatePacket.LocalAddr().String())
			if clientErr != nil || limited == nil {
				t.Fatalf("UDP client decode: %v", clientErr)
			}
			client.Net = "tcp"
			full, _, err := client.Exchange(q, gatePacket.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			if full.Truncated || len(full.Answer) != 12 {
				t.Fatalf("TCP retry lost records: %v", full)
			}
		})
	}
}
