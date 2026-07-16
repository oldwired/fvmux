package sftp

import "testing"

// TestRemoteParent pins the SFTP parent-directory semantics after the
// switch to path.Dir (#54): "/" and "" are their own parent, and trailing
// slashes are ignored so "/a/b/" resolves to "/a" like "/a/b" does —
// something bare path.Dir would not do.
func TestRemoteParent(t *testing.T) {
	cases := []struct {
		cwd  string
		want string
	}{
		{"", "/"},
		{"/", "/"},
		{"/a", "/"},
		{"/a/b", "/a"},
		{"/a/b/c", "/a/b"},
		{"/a/b/", "/a"},
		{"/a/b///", "/a"},
		{"//", "/"},
		{"/home/user", "/home"},
	}
	for _, c := range cases {
		if got := remoteParent(c.cwd); got != c.want {
			t.Errorf("remoteParent(%q) = %q; want %q", c.cwd, got, c.want)
		}
	}
	// The "../" row is suppressed only when parent == cwd, i.e. at root.
	if remoteParent("/") != "/" {
		t.Error("root must be its own parent so no ../ row is emitted")
	}
}

// TestJoinRemote pins joinRemote's root special-case, which the listing,
// tree, and dirops builders now all rely on instead of re-inlining it.
func TestJoinRemote(t *testing.T) {
	cases := []struct {
		cwd, name, want string
	}{
		{"/", "x", "/x"},
		{"", "x", "/x"},
		{"/a", "x", "/a/x"},
		{"/a/b", "x", "/a/b/x"},
	}
	for _, c := range cases {
		if got := joinRemote(c.cwd, c.name); got != c.want {
			t.Errorf("joinRemote(%q, %q) = %q; want %q", c.cwd, c.name, got, c.want)
		}
	}
}
