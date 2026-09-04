package sftp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"
)

type remoteInfo struct {
	name string
	dir  bool
}

func (i remoteInfo) Name() string { return i.name }
func (i remoteInfo) Size() int64  { return 0 }
func (i remoteInfo) Mode() os.FileMode {
	if i.dir {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (i remoteInfo) ModTime() time.Time { return time.Time{} }
func (i remoteInfo) IsDir() bool        { return i.dir }
func (i remoteInfo) Sys() any           { return nil }

func TestValidateRemoteEntryNamePortableSegment(t *testing.T) {
	t.Parallel()
	unsafe := []string{
		"", ".", "..", "a/b", `a\b`, "a\x00b", "C:escape", "note:stream",
		"trailing.", "trailing ", "CON", "con.txt", "PRN", "AUX.md", "NUL",
		"COM1.log", "LPT9",
	}
	for _, name := range unsafe {
		if err := validateRemoteEntryName(name); !errors.Is(err, ErrUnsafeRemoteName) {
			t.Errorf("validateRemoteEntryName(%q) = %v, want ErrUnsafeRemoteName", name, err)
		}
	}
	for _, name := range []string{"ordinary.txt", "space inside", "résumé.md", "COM10"} {
		if err := validateRemoteEntryName(name); err != nil {
			t.Errorf("validateRemoteEntryName(%q) = %v", name, err)
		}
	}
}

func TestSanitizeRemoteDirectoryCapsAndMarksEntries(t *testing.T) {
	t.Parallel()
	entries := make([]os.FileInfo, 0, maxRemoteEntriesPerDirectory+2)
	entries = append(entries, remoteInfo{name: `..\outside`})
	for i := 0; i < maxRemoteEntriesPerDirectory+1; i++ {
		entries = append(entries, remoteInfo{name: "file-" + strings.Repeat("x", i%8)})
	}
	d := sanitizeRemoteDirectory(entries, false)
	if len(d.entries) > maxRemoteEntriesPerDirectory {
		t.Fatalf("retained %d entries, limit %d", len(d.entries), maxRemoteEntriesPerDirectory)
	}
	if !d.truncated || d.rejected != 1 {
		t.Fatalf("sanitize result = truncated %v, rejected %d; want true, 1", d.truncated, d.rejected)
	}
}

func TestByteBudgetReaderStopsAtHardLimit(t *testing.T) {
	t.Parallel()
	want := int64(32)
	r := &byteBudgetReader{r: bytes.NewReader(bytes.Repeat([]byte{'x'}, int(want)+1)), remaining: want}
	got, err := io.ReadAll(r)
	if !errors.Is(err, ErrRemoteDirectoryLimit) {
		t.Fatalf("ReadAll error = %v, want ErrRemoteDirectoryLimit", err)
	}
	if int64(len(got)) != want || !r.exhausted.Load() {
		t.Fatalf("read %d bytes, exhausted=%v; want %d, true", len(got), r.exhausted.Load(), want)
	}
}

func TestWalkRemoteDepthLimitIsIterativeAndBounded(t *testing.T) {
	t.Parallel()
	calls := 0
	readDir := func(_ context.Context, _ string) (remoteDirectory, error) {
		calls++
		return remoteDirectory{entries: []os.FileInfo{remoteInfo{name: "next", dir: true}}}, nil
	}
	err := walkRemote(context.Background(), readDir, "/", func(string, string, bool) error { return nil })
	if !errors.Is(err, ErrRemoteTreeLimit) {
		t.Fatalf("walkRemote error = %v, want ErrRemoteTreeLimit", err)
	}
	if calls > maxTreeDepth+1 {
		t.Fatalf("walkRemote made %d directory reads for depth limit %d", calls, maxTreeDepth)
	}
}

func TestWalkRemoteCancellationInterruptsDirectoryRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- walkRemote(ctx, func(ctx context.Context, _ string) (remoteDirectory, error) {
			close(entered)
			<-ctx.Done()
			return remoteDirectory{}, ctx.Err()
		}, "/", func(string, string, bool) error { return nil })
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("walkRemote error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("walkRemote did not stop after cancellation")
	}
}

func TestRemoteTreeBudgetCapsAggregateNodes(t *testing.T) {
	t.Parallel()
	b := &remoteTreeBudget{limit: 2}
	nodes := func(names ...string) []*treeview.Node {
		out := make([]*treeview.Node, 0, len(names))
		for _, name := range names {
			out = append(out, &treeview.Node{Label: name, Data: &fileEntry{Path: "/" + name}})
		}
		return out
	}
	if got := b.accept(nodes("a")); countRemoteTreeNodes(got) != 1 {
		t.Fatalf("first accept retained %d nodes, want 1", countRemoteTreeNodes(got))
	}
	got := b.accept(nodes("b", "c"))
	if countRemoteTreeNodes(got) != 1 || got[len(got)-1].Label != "<tree node limit reached>" {
		t.Fatalf("second accept = %#v; want one data node plus limit marker", got)
	}
}
