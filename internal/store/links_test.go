package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newLinkStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "links.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestEnsureLinkIsStable(t *testing.T) {
	st := newLinkStore(t)
	ctx := context.Background()
	id, _ := st.Create(ctx, Account{Email: "a@x.com"})

	first, err := st.EnsureLink(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == "" {
		t.Fatal("empty token")
	}

	second, err := st.EnsureLink(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token != first.Token {
		t.Fatalf("token changed: %q -> %q", first.Token, second.Token)
	}
}

func TestLinkByTokenAndRevoke(t *testing.T) {
	st := newLinkStore(t)
	ctx := context.Background()
	id, _ := st.Create(ctx, Account{Email: "b@x.com"})

	link, _ := st.EnsureLink(ctx, id)
	got, err := st.LinkByToken(ctx, link.Token)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountID != id {
		t.Fatalf("account mismatch: %d", got.AccountID)
	}

	if err := st.RevokeLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LinkByToken(ctx, link.Token); err != ErrLinkNotFound {
		t.Fatalf("revoked token should be rejected, got %v", err)
	}

	// A fresh token must differ from the revoked one.
	fresh, err := st.EnsureLink(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Token == link.Token {
		t.Fatal("new token should differ after revoke")
	}
}

func TestUnknownToken(t *testing.T) {
	st := newLinkStore(t)
	if _, err := st.LinkByToken(context.Background(), "nope"); err != ErrLinkNotFound {
		t.Fatalf("want ErrLinkNotFound, got %v", err)
	}
}

func TestTouchLink(t *testing.T) {
	st := newLinkStore(t)
	ctx := context.Background()
	id, _ := st.Create(ctx, Account{Email: "c@x.com"})
	link, _ := st.EnsureLink(ctx, id)

	st.TouchLink(ctx, link.ID)
	st.TouchLink(ctx, link.ID)

	got, _ := st.LinkByToken(ctx, link.Token)
	if got.UseCount != 2 {
		t.Fatalf("use count = %d, want 2", got.UseCount)
	}
	if got.LastUsedAt == 0 {
		t.Fatal("last used not recorded")
	}
}

func TestLinksForAccounts(t *testing.T) {
	st := newLinkStore(t)
	ctx := context.Background()
	a, _ := st.Create(ctx, Account{Email: "d@x.com"})
	b, _ := st.Create(ctx, Account{Email: "e@x.com"})

	la, _ := st.EnsureLink(ctx, a)
	if _, err := st.EnsureLink(ctx, b); err != nil {
		t.Fatal(err)
	}

	m, err := st.LinksForAccounts(ctx, []int64{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("want 2 links, got %d", len(m))
	}
	if m[a].Token != la.Token {
		t.Fatal("wrong token for account a")
	}
}

func TestDeleteAccountRemovesLinks(t *testing.T) {
	st := newLinkStore(t)
	ctx := context.Background()
	id, _ := st.Create(ctx, Account{Email: "f@x.com"})
	link, _ := st.EnsureLink(ctx, id)

	if err := st.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LinkByToken(ctx, link.Token); err != ErrLinkNotFound {
		t.Fatalf("link should be gone with the account, got %v", err)
	}
}

func TestBulkDeleteRemovesLinks(t *testing.T) {
	st := newLinkStore(t)
	ctx := context.Background()
	a, _ := st.Create(ctx, Account{Email: "g@x.com"})
	b, _ := st.Create(ctx, Account{Email: "h@x.com"})
	la, _ := st.EnsureLink(ctx, a)
	lb, _ := st.EnsureLink(ctx, b)

	if _, err := st.DeleteMany(ctx, []int64{a, b}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LinkByToken(ctx, la.Token); err != ErrLinkNotFound {
		t.Fatal("link a should be gone")
	}
	if _, err := st.LinkByToken(ctx, lb.Token); err != ErrLinkNotFound {
		t.Fatal("link b should be gone")
	}
}
