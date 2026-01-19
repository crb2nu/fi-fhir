package workflow

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestEmailAction_SMTPHappyPath(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	var (
		mu      sync.Mutex
		gotData string
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		write := func(s string) {
			_, _ = w.WriteString(s + "\r\n")
			_ = w.Flush()
		}

		write("220 localhost ESMTP test")

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			upper := strings.ToUpper(line)

			switch {
			case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
				write("250-localhost")
				write("250 PIPELINING")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				write("250 OK")
			case strings.HasPrefix(upper, "RCPT TO:"):
				write("250 OK")
			case upper == "DATA":
				write("354 End data with <CR><LF>.<CR><LF>")
				var b strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				mu.Lock()
				gotData = b.String()
				mu.Unlock()
				write("250 OK")
			case upper == "QUIT":
				write("221 Bye")
				return
			default:
				write("250 OK")
			}
		}
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "test",
		},
		Patient: events.Patient{MRN: "123"},
	}

	cfg := map[string]string{
		"smtp_host": host,
		"smtp_port": port,
		"from":      "from@example.com",
		"to":        "to@example.com",
		"subject":   "Admit {{.Patient.MRN}}",
		"body":      "Event={{.Type}}",
		"timeout":   "5s",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := emailAction(ctx, event, cfg); err != nil {
		t.Fatalf("emailAction: %v", err)
	}

	<-done

	mu.Lock()
	defer mu.Unlock()
	if gotData == "" {
		t.Fatalf("expected DATA payload")
	}
	if !strings.Contains(gotData, "Subject: Admit 123") {
		t.Fatalf("missing subject header:\n%s", gotData)
	}
	if !strings.Contains(gotData, "From: from@example.com") {
		t.Fatalf("missing from header:\n%s", gotData)
	}
	if !strings.Contains(gotData, "To: to@example.com") {
		t.Fatalf("missing to header:\n%s", gotData)
	}
	if !strings.Contains(gotData, "Event=patient_admit") {
		t.Fatalf("missing body:\n%s", gotData)
	}
}
