package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestParseImportLine(t *testing.T) {
	cases := []struct {
		in       string
		wantOK   bool
		wantMail string
		wantApp  string
	}{
		{"a@gmail.com----pw----SECRET----apppass", true, "a@gmail.com", "apppass"},
		{"  b@gmail.com ---- pw ---- secret ---- app pass ", true, "b@gmail.com", "app pass"},
		{"c@gmail.com\tpw\tsecret\tapppass", true, "c@gmail.com", "apppass"},
		{"d@gmail.com|pw|secret|apppass", true, "d@gmail.com", "apppass"},
		{"e@gmail.com,pw,secret,apppass", true, "e@gmail.com", "apppass"},
		{"f@gmail.com----pw----secret", true, "f@gmail.com", ""},
		{"# comment", false, "", ""},
		{"", false, "", ""},
		{"nonsense", false, "", ""},
		{"----pw----secret----app", false, "", ""},
		{"\ufeffg@gmail.com----pw----s----a", true, "g@gmail.com", "a"},
	}
	for _, c := range cases {
		fields, ok := ParseImportLine(c.in)
		if ok != c.wantOK {
			t.Errorf("ParseImportLine(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if fields[0] != c.wantMail {
			t.Errorf("ParseImportLine(%q) email = %q, want %q", c.in, fields[0], c.wantMail)
		}
		if fields[3] != c.wantApp {
			t.Errorf("ParseImportLine(%q) app = %q, want %q", c.in, fields[3], c.wantApp)
		}
	}
}

func TestFormatExportLineRoundTrip(t *testing.T) {
	line := FormatExportLine("a@gmail.com", "pw", "SECRET", "apppass")
	fields, ok := ParseImportLine(line)
	if !ok {
		t.Fatal("export line should parse")
	}
	if fields[0] != "a@gmail.com" || fields[1] != "pw" || fields[2] != "SECRET" || fields[3] != "apppass" {
		t.Fatalf("round trip mismatch: %#v", fields)
	}
}

func TestCRUDAndPagination(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	for i, email := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		if _, err := st.Create(ctx, Account{Email: email, AppPassword: "enc", Note: "n"}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := st.List(ctx, ListParams{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("page1: total=%d len=%d", total, len(items))
	}
	items2, _, _ := st.List(ctx, ListParams{Page: 2, PageSize: 2})
	if len(items2) != 1 {
		t.Fatalf("page2 len=%d", len(items2))
	}

	found, total, err := st.List(ctx, ListParams{Page: 1, PageSize: 10, Query: "b@"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(found) != 1 || found[0].Email != "b@x.com" {
		t.Fatalf("search mismatch: %+v", found)
	}

	got, err := st.Get(ctx, items[0].ID)
	if err != nil || got.Email != items[0].Email {
		t.Fatalf("get: %v %+v", err, got)
	}

	if err := st.Update(ctx, Account{ID: got.ID, Email: "renamed@x.com", AppPassword: "enc2", Note: "x"}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get(ctx, got.ID)
	if got.Email != "renamed@x.com" || got.AppPassword != "enc2" {
		t.Fatalf("update failed: %+v", got)
	}

	if err := st.Delete(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, got.ID); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDuplicateEmail(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	if _, err := st.Create(ctx, Account{Email: "dup@x.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, Account{Email: "DUP@x.com"}); err != ErrDuplicate {
		t.Fatalf("want ErrDuplicate (case-insensitive), got %v", err)
	}
}

func TestUpsert(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	inserted, err := st.Upsert(ctx, Account{Email: "u@x.com", AppPassword: "one"}, false)
	if err != nil || !inserted {
		t.Fatalf("first upsert: inserted=%v err=%v", inserted, err)
	}
	inserted, err = st.Upsert(ctx, Account{Email: "u@x.com", AppPassword: "two"}, false)
	if err != nil || inserted {
		t.Fatalf("second upsert should skip: inserted=%v err=%v", inserted, err)
	}
	items, _, _ := st.List(ctx, ListParams{Page: 1, PageSize: 10})
	if items[0].AppPassword != "one" {
		t.Fatalf("skip should not change data, got %q", items[0].AppPassword)
	}

	inserted, err = st.Upsert(ctx, Account{Email: "u@x.com", AppPassword: "three"}, true)
	if err != nil || inserted {
		t.Fatalf("overwrite upsert: inserted=%v err=%v", inserted, err)
	}
	items, _, _ = st.List(ctx, ListParams{Page: 1, PageSize: 10})
	if items[0].AppPassword != "three" {
		t.Fatalf("overwrite failed, got %q", items[0].AppPassword)
	}
}

func TestSetFetchResultAndStats(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	id, _ := st.Create(ctx, Account{Email: "s@x.com", AppPassword: "enc"})
	id2, _ := st.Create(ctx, Account{Email: "s2@x.com"})

	if err := st.SetFetchResult(ctx, MarkFetch{ID: id, Status: "ok", Subject: "Hi", From: "a@b.c", Date: 123}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFetchResult(ctx, MarkFetch{ID: id2, Status: "error", Error: "boom"}); err != nil {
		t.Fatal(err)
	}

	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.WithAppPass != 1 || stats.FetchOK != 1 || stats.FetchFailed != 1 || stats.FetchedToday != 2 {
		t.Fatalf("stats mismatch: %+v", stats)
	}

	got, _ := st.Get(ctx, id)
	if got.LastFetchSubject != "Hi" || got.LastFetchStatus != "ok" || got.LastFetchDate != 123 {
		t.Fatalf("fetch result not stored: %+v", got)
	}
}

func TestDeleteMany(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	a, _ := st.Create(ctx, Account{Email: "1@x.com"})
	b, _ := st.Create(ctx, Account{Email: "2@x.com"})
	_, _ = st.Create(ctx, Account{Email: "3@x.com"})

	n, err := st.DeleteMany(ctx, []int64{a, b})
	if err != nil || n != 2 {
		t.Fatalf("deleted=%d err=%v", n, err)
	}
	_, total, _ := st.List(ctx, ListParams{Page: 1, PageSize: 10})
	if total != 1 {
		t.Fatalf("total=%d", total)
	}
}
