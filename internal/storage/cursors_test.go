package storage_test

import (
	"context"
	"testing"
)

func TestGetCursorUnknownReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	offset, inode, err := s.GetCursor(ctx, "work", "/no/such/file.jsonl")
	if err != nil {
		t.Fatalf("GetCursor unknown file: %v", err)
	}
	if offset != 0 || inode != 0 {
		t.Errorf("unknown cursor: got (%d, %d), want (0, 0)", offset, inode)
	}
}

func TestSetAndGetCursorRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	path := "/Users/arafa/.claude-profiles/work/projects/ccx/sess.jsonl"

	if err := s.SetCursor(ctx, "work", path, 4096, 0xDEADBEEF); err != nil {
		t.Fatalf("SetCursor: %v", err)
	}

	offset, inode, err := s.GetCursor(ctx, "work", path)
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if offset != 4096 {
		t.Errorf("offset: got %d, want 4096", offset)
	}
	if inode != 0xDEADBEEF {
		t.Errorf("inode: got %x, want %x", inode, uint64(0xDEADBEEF))
	}
}

func TestSetCursorUpsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	path := "/a/b/c.jsonl"
	if err := s.SetCursor(ctx, "work", path, 100, 1); err != nil {
		t.Fatalf("first SetCursor: %v", err)
	}
	if err := s.SetCursor(ctx, "work", path, 200, 2); err != nil {
		t.Fatalf("second SetCursor: %v", err)
	}
	offset, inode, err := s.GetCursor(ctx, "work", path)
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if offset != 200 || inode != 2 {
		t.Errorf("upserted cursor: got (%d, %d), want (200, 2)", offset, inode)
	}
}

func TestCursorIsolatedPerProfile(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	mustSaveProfile(t, s, "personal")

	path := "/shared/path.jsonl"
	if err := s.SetCursor(ctx, "work", path, 100, 1); err != nil {
		t.Fatalf("SetCursor(work): %v", err)
	}
	if err := s.SetCursor(ctx, "personal", path, 999, 2); err != nil {
		t.Fatalf("SetCursor(personal): %v", err)
	}

	workOffset, _, _ := s.GetCursor(ctx, "work", path)
	persOffset, _, _ := s.GetCursor(ctx, "personal", path)
	if workOffset != 100 || persOffset != 999 {
		t.Errorf("isolation: work=%d personal=%d, want 100/999", workOffset, persOffset)
	}
}
