package service

import (
	"context"
	"testing"
	"time"
)

func TestSleepCtxRespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	err := sleepCtx(ctx, time.Second)
	if err == nil {
		t.Fatal("expected context error")
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatalf("sleepCtx ignored cancel, took %s", time.Since(started))
	}
}

func TestSleepCtxWaits(t *testing.T) {
	started := time.Now()
	err := sleepCtx(context.Background(), 20*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(started) < 20*time.Millisecond {
		t.Fatal("sleepCtx returned too early")
	}
}

func TestWakeMonitorNonBlocking(t *testing.T) {
	s := &service{}
	ch := make(chan struct{}, 1)
	s.commandNotify.Store("aabbccddeeff", ch)

	s.wakeMonitor("aabbccddeeff")
	s.wakeMonitor("aabbccddeeff")
	s.wakeMonitor("missing")

	if len(ch) != 1 {
		t.Fatalf("expected 1 queued wake, got %d", len(ch))
	}
}

func TestOnGetFailureMarksOfflineAfterThreeFails(t *testing.T) {
	s := &service{}
	m := &deviceMonitor{
		s:            s,
		mac:          "aabbccddeeff",
		offlineAfter: time.Hour,
		hasSuccess:   true,
		isOnline:     true,
	}

	m.onGetFailure(context.Background())
	m.onGetFailure(context.Background())
	if m.consecutiveFails != 2 {
		t.Fatalf("consecutiveFails=%d, want 2", m.consecutiveFails)
	}
	if m.consecutiveFails >= offlineFailCount {
		t.Fatal("device should stay online after two failed gets")
	}

	m.consecutiveFails++
	if m.consecutiveFails < offlineFailCount {
		t.Fatalf("consecutiveFails=%d, want >= %d", m.consecutiveFails, offlineFailCount)
	}
}

func TestSleepBackoffSequence(t *testing.T) {
	m := &deviceMonitor{backoffCap: 30 * time.Second}

	got := make([]time.Duration, 0, 4)
	for i := 0; i < 4; i++ {
		d := minBackoff << m.backoffStep
		if d <= 0 || d > m.backoffCap {
			d = m.backoffCap
		}
		if m.backoffStep < 16 {
			m.backoffStep++
		}
		got = append(got, d)
	}

	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff[%d]=%s, want %s", i, got[i], want[i])
		}
	}
}

func TestWaitMinGapSkipsWhenNoPriorUDP(t *testing.T) {
	m := &deviceMonitor{}
	started := time.Now()
	if err := m.waitMinGap(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("waitMinGap should not sleep before the first UDP")
	}
}
