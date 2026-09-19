// Copyright 2026 Optiqor contributors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"testing"
)

// fakeCollector implements Collector for testing.
type fakeCollector struct {
	name    string
	started bool
	stopped bool
	snap    any
	startFn func() error
}

func (f *fakeCollector) Name() string { return f.name }

func (f *fakeCollector) Start(_ context.Context) error {
	f.started = true
	if f.startFn != nil {
		return f.startFn()
	}
	return nil
}

func (f *fakeCollector) Stop() { f.stopped = true }

func (f *fakeCollector) Snapshot() any { return f.snap }

// ─── Registry Tests ─────────────────────────────────────────────────────────

func newTestRegistry() *Registry {
	return NewRegistry(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

// runRegistryCases executes table-driven registry cases. Each case gets its
// own registry so subtests stay independent (per issue #135).
func runRegistryCases(t *testing.T, tests []struct {
	name string
	run  func(t *testing.T, r *Registry)
}) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, newTestRegistry())
		})
	}
}

func TestRegistryRegister(t *testing.T) {
	runRegistryCases(t, []struct {
		name string
		run  func(t *testing.T, r *Registry)
	}{
		{
			"register and get returns same collector",
			func(t *testing.T, r *Registry) {
				c := &fakeCollector{name: "test"}
				if err := r.Register(c); err != nil {
					t.Fatalf("Register() unexpected error: %v", err)
				}
				if got := r.Get("test"); got != c {
					t.Errorf("Get('test') returned wrong collector")
				}
			},
		},
		{
			"duplicate name rejected",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "dup"}); err != nil {
					t.Fatalf("Register(c1) unexpected error: %v", err)
				}
				if err := r.Register(&fakeCollector{name: "dup"}); err == nil {
					t.Error("Register(c2) expected error for duplicate, got nil")
				}
			},
		},
		{
			"get unknown on empty registry returns nil",
			func(t *testing.T, r *Registry) {
				if got := r.Get("nonexistent"); got != nil {
					t.Errorf("Get('nonexistent') = %v, want nil", got)
				}
			},
		},
	})
}

func TestRegistryNames(t *testing.T) {
	runRegistryCases(t, []struct {
		name string
		run  func(t *testing.T, r *Registry)
	}{
		{
			"empty registry has no names",
			func(t *testing.T, r *Registry) {
				if names := r.Names(); len(names) != 0 {
					t.Errorf("Names() on empty registry = %v, want none", names)
				}
			},
		},
		{
			"single collector",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "alpha"}); err != nil {
					t.Fatal(err)
				}
				names := r.Names()
				if len(names) != 1 || names[0] != "alpha" {
					t.Errorf("Names() = %v, want [alpha]", names)
				}
			},
		},
		{
			"multiple collectors sorted",
			func(t *testing.T, r *Registry) {
				for _, name := range []string{"charlie", "alpha", "bravo"} {
					if err := r.Register(&fakeCollector{name: name}); err != nil {
						t.Fatalf("Register(%q): %v", name, err)
					}
				}
				names := r.Names()
				sort.Strings(names)
				want := []string{"alpha", "bravo", "charlie"}
				if len(names) != len(want) {
					t.Fatalf("Names() returned %d names, want %d", len(names), len(want))
				}
				for i, n := range names {
					if n != want[i] {
						t.Errorf("Names()[%d] = %q, want %q", i, n, want[i])
					}
				}
			},
		},
	})
}

func TestRegistryStartAll(t *testing.T) {
	runRegistryCases(t, []struct {
		name string
		run  func(t *testing.T, r *Registry)
	}{
		{
			"empty registry starts cleanly",
			func(t *testing.T, r *Registry) {
				if err := r.StartAll(context.Background()); err != nil {
					t.Errorf("StartAll() on empty registry unexpected error: %v", err)
				}
			},
		},
		{
			"single collector started",
			func(t *testing.T, r *Registry) {
				c := &fakeCollector{name: "solo"}
				if err := r.Register(c); err != nil {
					t.Fatal(err)
				}
				if err := r.StartAll(context.Background()); err != nil {
					t.Fatalf("StartAll() unexpected error: %v", err)
				}
				if !c.started {
					t.Errorf("collector %q was not started", c.name)
				}
			},
		},
		{
			"all collectors started",
			func(t *testing.T, r *Registry) {
				collectors := []*fakeCollector{{name: "a"}, {name: "b"}}
				for _, c := range collectors {
					if err := r.Register(c); err != nil {
						t.Fatal(err)
					}
				}
				if err := r.StartAll(context.Background()); err != nil {
					t.Fatalf("StartAll() unexpected error: %v", err)
				}
				for _, c := range collectors {
					if !c.started {
						t.Errorf("collector %q was not started", c.name)
					}
				}
			},
		},
		{
			"start error propagates",
			func(t *testing.T, r *Registry) {
				bad := &fakeCollector{
					name: "fail",
					startFn: func() error {
						return context.DeadlineExceeded
					},
				}
				if err := r.Register(bad); err != nil {
					t.Fatal(err)
				}
				if err := r.StartAll(context.Background()); err == nil {
					t.Error("StartAll() expected error, got nil")
				}
			},
		},
	})
}

func TestRegistryStopAll(t *testing.T) {
	runRegistryCases(t, []struct {
		name string
		run  func(t *testing.T, r *Registry)
	}{
		{
			"empty registry stops cleanly",
			func(t *testing.T, r *Registry) {
				r.StopAll() // must not panic
			},
		},
		{
			"all collectors stopped",
			func(t *testing.T, r *Registry) {
				collectors := []*fakeCollector{{name: "x"}, {name: "y"}}
				for _, c := range collectors {
					if err := r.Register(c); err != nil {
						t.Fatal(err)
					}
				}
				r.StopAll()
				for _, c := range collectors {
					if !c.stopped {
						t.Errorf("collector %q was not stopped", c.name)
					}
				}
			},
		},
	})
}

func TestRegistrySignals(t *testing.T) {
	runRegistryCases(t, []struct {
		name string
		run  func(t *testing.T, r *Registry)
	}{
		{
			"empty registry yields all nil snapshots",
			func(t *testing.T, r *Registry) {
				s := r.Signals(30_000_000_000)
				if s.Syscall != nil || s.TCP != nil || s.OOM != nil || s.DiskIO != nil ||
					s.Sched != nil || s.FD != nil || s.Memory != nil || s.CgroupMemory != nil {
					t.Errorf("Signals() on empty registry returned non-nil snapshots: %+v", s)
				}
			},
		},
		{
			"nil snapshot ignored",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "empty", snap: nil}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.Syscall != nil || s.TCP != nil || s.OOM != nil || s.DiskIO != nil ||
					s.Sched != nil || s.FD != nil || s.Memory != nil || s.CgroupMemory != nil {
					t.Errorf("nil snapshot should be skipped, got: %+v", s)
				}
			},
		},
		{
			"syscall snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "syscall", snap: &SyscallSnapshot{TotalCount: 1000}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.Syscall == nil || s.Syscall.TotalCount != 1000 {
					t.Errorf("Signals.Syscall = %+v, want TotalCount=1000", s.Syscall)
				}
			},
		},
		{
			"tcp snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "tcp", snap: &TCPSnapshot{ActiveConnections: 5}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.TCP == nil || s.TCP.ActiveConnections != 5 {
					t.Errorf("Signals.TCP = %+v, want ActiveConnections=5", s.TCP)
				}
			},
		},
		{
			"oom snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "oom", snap: &OOMSnapshot{Count: 2}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.OOM == nil || s.OOM.Count != 2 {
					t.Errorf("Signals.OOM = %+v, want Count=2", s.OOM)
				}
			},
		},
		{
			"disk io snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "disk", snap: &DiskIOSnapshot{TotalReads: 42}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.DiskIO == nil || s.DiskIO.TotalReads != 42 {
					t.Errorf("Signals.DiskIO = %+v, want TotalReads=42", s.DiskIO)
				}
			},
		},
		{
			"sched snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "sched", snap: &SchedSnapshot{TotalCount: 7}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.Sched == nil || s.Sched.TotalCount != 7 {
					t.Errorf("Signals.Sched = %+v, want TotalCount=7", s.Sched)
				}
			},
		},
		{
			"fd snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "fd", snap: &FDSnapshot{TotalOpens: 9}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.FD == nil || s.FD.TotalOpens != 9 {
					t.Errorf("Signals.FD = %+v, want TotalOpens=9", s.FD)
				}
			},
		},
		{
			"memory snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "mem", snap: &MemorySnapshot{TotalBytes: 1024}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.Memory == nil || s.Memory.TotalBytes != 1024 {
					t.Errorf("Signals.Memory = %+v, want TotalBytes=1024", s.Memory)
				}
			},
		},
		{
			"cgroup memory snapshot",
			func(t *testing.T, r *Registry) {
				if err := r.Register(&fakeCollector{name: "cgm", snap: &CgroupMemorySnapshot{
					Containers: []CgroupMemoryEntry{{CgroupPath: "/kubepods/pod1"}},
				}}); err != nil {
					t.Fatal(err)
				}
				s := r.Signals(30_000_000_000)
				if s.CgroupMemory == nil || len(s.CgroupMemory.Containers) != 1 {
					t.Errorf("Signals.CgroupMemory = %+v, want 1 container", s.CgroupMemory)
				}
			},
		},
		{
			"mixed collectors fill only their fields",
			func(t *testing.T, r *Registry) {
				collectors := []Collector{
					&fakeCollector{name: "syscall", snap: &SyscallSnapshot{TotalCount: 1000}},
					&fakeCollector{name: "tcp", snap: &TCPSnapshot{ActiveConnections: 5}},
					&fakeCollector{name: "oom", snap: &OOMSnapshot{Count: 2}},
					&fakeCollector{name: "empty", snap: nil}, // nil snapshot
				}
				for _, c := range collectors {
					if err := r.Register(c); err != nil {
						t.Fatal(err)
					}
				}
				s := r.Signals(30_000_000_000) // 30s
				if s.Syscall == nil || s.Syscall.TotalCount != 1000 {
					t.Errorf("Signals.Syscall = %+v, want TotalCount=1000", s.Syscall)
				}
				if s.TCP == nil || s.TCP.ActiveConnections != 5 {
					t.Errorf("Signals.TCP = %+v, want ActiveConnections=5", s.TCP)
				}
				if s.OOM == nil || s.OOM.Count != 2 {
					t.Errorf("Signals.OOM = %+v, want Count=2", s.OOM)
				}
				if s.DiskIO != nil {
					t.Errorf("Signals.DiskIO should be nil, got %+v", s.DiskIO)
				}
				if s.Sched != nil {
					t.Errorf("Signals.Sched should be nil, got %+v", s.Sched)
				}
				if s.FD != nil {
					t.Errorf("Signals.FD should be nil, got %+v", s.FD)
				}
			},
		},
	})
}
