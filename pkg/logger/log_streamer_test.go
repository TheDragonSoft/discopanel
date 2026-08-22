package logger

import (
	"sync"
	"testing"
	"time"

	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLogStreamer_ConcurrentSubscribeBroadcastUnsubscribe(t *testing.T) {
	log := New()
	ls := NewLogStreamer(nil, log, 100)
	containerID := "test-container-1"

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 5 subscriber workers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					ch := ls.Subscribe(containerID)
					time.Sleep(1 * time.Millisecond)
					// Read from ch if available
					select {
					case <-ch:
					default:
					}
					ls.Unsubscribe(containerID, ch)
				}
			}
		}()
	}

	// 3 broadcaster workers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					entry := &v1.LogEntry{
						Timestamp: timestamppb.Now(),
						Message:   "test message",
						Level:     "info",
					}
					ls.broadcast(containerID, entry)
					time.Sleep(500 * time.Microsecond)
				}
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestLogStreamer_DoubleUnsubscribeSafety(t *testing.T) {
	log := New()
	ls := NewLogStreamer(nil, log, 100)
	containerID := "test-container-2"

	ch := ls.Subscribe(containerID)

	// First unsubscribe should close channel safely
	ls.Unsubscribe(containerID, ch)

	// Second unsubscribe on the same channel should NOT panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic on double unsubscribe: %v", r)
		}
	}()
	ls.Unsubscribe(containerID, ch)

	// Unsubscribe on non-existent container should NOT panic
	ls.Unsubscribe("non-existent-container", ch)
}

func TestLogStreamer_MigrateSubscribers(t *testing.T) {
	log := New()
	ls := NewLogStreamer(nil, log, 100)
	oldID := "container-old"
	newID := "container-new"

	ch := ls.Subscribe(oldID)

	// Migrate from old to new
	ls.MigrateSubscribers(oldID, newID)

	// Broadcast to new container ID
	entry := &v1.LogEntry{
		Timestamp: timestamppb.Now(),
		Message:   "migrated log message",
		Level:     "info",
	}
	ls.broadcast(newID, entry)

	select {
	case received := <-ch:
		if received.Message != "migrated log message" {
			t.Fatalf("expected 'migrated log message', got '%s'", received.Message)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for log entry on migrated subscriber")
	}

	// Unsubscribe from new container ID
	ls.Unsubscribe(newID, ch)
}

func TestLogStreamer_AddCommandEntry(t *testing.T) {
	log := New()
	ls := NewLogStreamer(nil, log, 100)
	containerID := "test-container-cmd"

	ch := ls.Subscribe(containerID)
	defer ls.Unsubscribe(containerID, ch)

	ls.AddCommandEntry(containerID, "say hello", time.Now())

	select {
	case received := <-ch:
		if !received.IsCommand {
			t.Fatal("expected IsCommand to be true")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for command entry")
	}
}
