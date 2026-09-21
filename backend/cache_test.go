package backend_test

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clobrano/jira-tabbed-tui/backend"
)

// runCmd executes a tea.Cmd synchronously and returns the resulting message.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestFetchChildrenUsesSupportedPageSize(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "list.json"), "issue", "list", "--raw", "-q", "project = PROJ AND parent = PROJ-123", "--paginate", "0:100")

	msg := runCmd(backend.FetchChildrenCmd(r, "PROJ-123"))
	cm, ok := msg.(backend.ChildrenFetchedMsg)
	if !ok {
		t.Fatalf("expected ChildrenFetchedMsg, got %T", msg)
	}
	if cm.Err != nil {
		t.Fatalf("unexpected error: %v", cm.Err)
	}
	if len(cm.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(cm.Children))
	}
}

func TestCacheTTLNotExpired(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "list.json"), "issue", "list", "--raw", "-q", "project = A", "--paginate", "0:50")

	c := backend.NewCache()

	// First fetch populates the cache.
	msg1 := runCmd(c.FetchListCmd(r, 0, "tabA", "project = A"))
	lm1, ok := msg1.(backend.ListFetchedMsg)
	if !ok {
		t.Fatalf("expected ListFetchedMsg, got %T", msg1)
	}
	if lm1.Err != nil {
		t.Fatalf("unexpected error: %v", lm1.Err)
	}

	// Deregister the runner response to simulate no network.
	r2 := backend.NewFakeRunner() // empty runner — any call would fail

	// Second fetch within TTL should return cached data, not call the runner.
	msg2 := runCmd(c.FetchListCmd(r2, 0, "tabA", "project = A"))
	lm2, ok := msg2.(backend.ListFetchedMsg)
	if !ok {
		t.Fatalf("expected ListFetchedMsg, got %T", msg2)
	}
	if lm2.Err != nil {
		t.Fatalf("cache hit returned error: %v", lm2.Err)
	}
	if len(lm2.Issues) != len(lm1.Issues) {
		t.Errorf("expected %d cached issues, got %d", len(lm1.Issues), len(lm2.Issues))
	}
}

func TestCacheTTLExpired(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "list.json"), "issue", "list", "--raw", "-q", "project = B", "--paginate", "0:50")

	c := backend.NewCacheWithTTL(1 * time.Millisecond)

	// Populate cache.
	runCmd(c.FetchListCmd(r, 0, "tabB", "project = B"))

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	// Register an error to confirm a real fetch is attempted after TTL.
	rFail := backend.NewFakeRunner()
	rFail.RegisterError(errors.New("network gone"), "issue", "list", "--raw", "-q", "project = B", "--paginate", "0:50")

	msg := runCmd(c.FetchListCmd(rFail, 0, "tabB", "project = B"))
	lm, ok := msg.(backend.ListFetchedMsg)
	if !ok {
		t.Fatalf("expected ListFetchedMsg, got %T", msg)
	}
	if lm.Err == nil {
		t.Fatal("expected error after TTL expiry with failing runner, got nil")
	}
	if !lm.Stale {
		t.Error("expected Stale=true when fetch fails but cached data exists")
	}
}

func TestCacheInvalidate(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "list.json"), "issue", "list", "--raw", "-q", "project = C", "--paginate", "0:50")
	r.Register(loadFixture(t, "detail.json"), "issue", "view", "PROJ-123", "--raw")

	c := backend.NewCache()

	// Populate tab and issue caches.
	runCmd(c.FetchListCmd(r, 0, "tabC", "project = C"))
	runCmd(c.FetchDetailCmd(r, "PROJ-123"))

	// Invalidate both.
	c.Invalidate("tabC", "PROJ-123")

	// After invalidation, an empty runner should cause a fetch attempt (which fails).
	rEmpty := backend.NewFakeRunner()
	msg := runCmd(c.FetchListCmd(rEmpty, 0, "tabC", "project = C"))
	lm := msg.(backend.ListFetchedMsg)
	if lm.Err == nil {
		t.Error("expected fetch to be attempted after invalidation")
	}

	msg2 := runCmd(c.FetchDetailCmd(rEmpty, "PROJ-123"))
	dm := msg2.(backend.DetailFetchedMsg)
	if dm.Err == nil {
		t.Error("expected detail fetch to be attempted after invalidation")
	}
}

func TestCacheForceRefreshBypassesTTL(t *testing.T) {
	r := backend.NewFakeRunner()
	r.Register(loadFixture(t, "list.json"), "issue", "list", "--raw", "-q", "project = D", "--paginate", "0:50")

	c := backend.NewCache()

	// Populate cache.
	runCmd(c.FetchListCmd(r, 0, "tabD", "project = D"))

	// Force refresh should bypass TTL and attempt a real fetch.
	rFail := backend.NewFakeRunner()
	rFail.RegisterError(errors.New("forced fail"), "issue", "list", "--raw", "-q", "project = D", "--paginate", "0:50")

	msg := runCmd(c.ForceFetchListCmd(rFail, 0, "tabD", "project = D"))
	lm := msg.(backend.ListFetchedMsg)
	if lm.Err == nil {
		t.Fatal("expected error from forced fetch, got nil — TTL was not bypassed")
	}
}
