//go:build test

package ui

import (
	"sync/atomic"
	"testing"
)

func TestSendLatestCountsDrops(t *testing.T) {
	ch := make(chan int, 1)
	var drops atomic.Int64
	ch <- 42
	sendLatest(ch, 99, &drops)
	if drops.Load() != 1 {
		t.Fatalf("expected 1 drop, got %d", drops.Load())
	}
	got := <-ch
	if got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
}

func TestSendLatestNilDropsCounter(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 42
	sendLatest(ch, 99, nil)
	got := <-ch
	if got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
}

func TestSendLatestNoDropWhenEmpty(t *testing.T) {
	ch := make(chan int, 1)
	var drops atomic.Int64
	sendLatest(ch, 7, &drops)
	if drops.Load() != 0 {
		t.Fatalf("expected 0 drops, got %d", drops.Load())
	}
	got := <-ch
	if got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}
