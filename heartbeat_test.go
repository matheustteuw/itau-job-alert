package main

import (
	"testing"
	"time"
)

func TestShouldSendHeartbeat(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	interval := 24 * time.Hour

	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{"nunca mandou heartbeat (zero value)", time.Time{}, true},
		{"mandou faz 25h, ja passou do intervalo", now.Add(-25 * time.Hour), true},
		{"mandou exatamente faz 24h", now.Add(-24 * time.Hour), true},
		{"mandou faz 1h, ainda nao passou", now.Add(-1 * time.Hour), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldSendHeartbeat(c.last, now, interval)
			if got != c.want {
				t.Errorf("shouldSendHeartbeat(%v, %v, %v) = %v, want %v", c.last, now, interval, got, c.want)
			}
		})
	}
}

func TestDecodeStateBackwardCompat(t *testing.T) {
	// Formato antigo: só o mapa de IDs vistos, sem heartbeat.
	legacy := []byte(`{"123": true, "456": true}`)
	st, err := decodeState(legacy)
	if err != nil {
		t.Fatalf("decodeState(legacy) erro: %v", err)
	}
	if len(st.Seen) != 2 || !st.Seen["123"] || !st.Seen["456"] {
		t.Errorf("decodeState(legacy) Seen = %v, want map com 123 e 456", st.Seen)
	}
	if !st.LastHeartbeat.IsZero() {
		t.Errorf("decodeState(legacy) LastHeartbeat deveria ser zero, veio %v", st.LastHeartbeat)
	}

	// Formato novo.
	modern := []byte(`{"seen": {"789": true}, "last_heartbeat": "2026-07-07T12:00:00Z"}`)
	st2, err := decodeState(modern)
	if err != nil {
		t.Fatalf("decodeState(modern) erro: %v", err)
	}
	if len(st2.Seen) != 1 || !st2.Seen["789"] {
		t.Errorf("decodeState(modern) Seen = %v, want map com 789", st2.Seen)
	}
	if st2.LastHeartbeat.IsZero() {
		t.Errorf("decodeState(modern) LastHeartbeat não deveria ser zero")
	}
}
