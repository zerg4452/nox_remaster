package gui

import "testing"

// Entry fields can own detached IME popup trees. Their destruction callback
// must run even though normal dispatch is disabled once Destroy marks them.
func TestWindowReloadDestroyCallback(t *testing.T) {
	g := New(nil)
	defer func() { g.DestroyAll(); g.alloc.Free() }()
	for cycle := 0; cycle < 30; cycle++ {
		calls := 0
		var entries []*Window
		root := g.NewWindowRaw(nil, StatusEnabled, 0, 0, 1, 1, nil)
		for i := 0; i < 2; i++ {
			popup := g.NewWindowRaw(nil, StatusHidden, 0, 0, 1, 1, nil)
			for j := 0; j < 4; j++ {
				g.NewWindowRaw(popup, StatusEnabled, 0, 0, 1, 1, nil)
			}
			entry := g.NewWindowRaw(root, StatusEnabled, 0, 0, 1, 1, func(_ *Window, ev WindowEvent) WindowEventResp {
				if _, ok := ev.(WindowDestroy); ok {
					calls++
					popup.Destroy()
				}
				return nil
			})
			entries = append(entries, entry)
		}
		root.Destroy()
		for _, entry := range entries {
			entry.Func94(WindowDestroy{})
		}
		if calls != 0 {
			t.Fatal("public dispatch reached a destroyed window")
		}
		g.FreeDestroyed()
		g.FreeDestroyed() // Reclaim detached trees enqueued by destruction callbacks.
		g.FreeDestroyed() // Repeated reclamation must not call destructors twice.
		if calls != 2 {
			t.Fatalf("cycle %d: got %d destruction callbacks, want 2", cycle, calls)
		}
		if g.head != nil || g.last != nil || g.free != nil {
			t.Fatalf("cycle %d: detached windows retained", cycle)
		}
	}
}

// Model HUD replacement: several independent trees, explicit destruction of
// child aliases after the parent, then deferred reclamation at frame end.
func TestWindowReloadReclaimsTrees(t *testing.T) {
	g := New(nil)
	defer func() { g.DestroyAll(); g.alloc.Free() }()
	for cycle := 0; cycle < 30; cycle++ {
		var roots, aliases []*Window
		for i := 0; i < 5; i++ {
			root := g.NewWindowRaw(nil, StatusEnabled, 0, 0, 100, 100, nil)
			roots = append(roots, root)
			for j := 0; j < 5; j++ {
				child := g.NewWindowRaw(root, StatusEnabled, 0, 0, 10, 10, nil)
				aliases = append(aliases, child)
				g.NewWindowRaw(child, StatusEnabled, 0, 0, 1, 1, nil)
			}
		}
		for _, root := range roots {
			root.Destroy()
		}
		for _, child := range aliases {
			child.Destroy()
		}
		g.FreeDestroyed()
		if g.head != nil || g.last != nil || g.free != nil {
			t.Fatalf("cycle %d: window lists retained after reclamation", cycle)
		}
		for _, root := range roots {
			if root.ext() != nil {
				t.Fatalf("cycle %d: extension retained", cycle)
			}
		}
	}
}
