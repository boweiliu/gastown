package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/worker"
)

func TestEffectivePolecatState(t *testing.T) {
	tests := []struct {
		name string
		item PolecatListItem
		want worker.State
	}{
		{
			name: "session-running-done-becomes-working",
			item: PolecatListItem{
				State:          worker.StateDone,
				SessionRunning: true,
			},
			want: worker.StateWorking,
		},
		{
			name: "session-dead-working-becomes-stalled",
			item: PolecatListItem{
				State:          worker.StateWorking,
				SessionRunning: false,
			},
			want: worker.StateStalled,
		},
		{
			name: "zombie-is-never-rewritten",
			item: PolecatListItem{
				State:          worker.StateZombie,
				SessionRunning: false,
				Zombie:         true,
			},
			want: worker.StateZombie,
		},
		{
			name: "idle-session-dead-stays-idle",
			item: PolecatListItem{
				State:          worker.StateIdle,
				SessionRunning: false,
			},
			want: worker.StateIdle,
		},
		{
			name: "idle-session-running-becomes-working",
			item: PolecatListItem{
				State:          worker.StateIdle,
				SessionRunning: true,
			},
			want: worker.StateWorking,
		},
		{
			name: "stalled-stays-stalled-when-session-dead",
			item: PolecatListItem{
				State:          worker.StateStalled,
				SessionRunning: false,
			},
			want: worker.StateStalled,
		},
		{
			name: "stalled-becomes-working-when-session-alive",
			item: PolecatListItem{
				State:          worker.StateStalled,
				SessionRunning: true,
			},
			want: worker.StateStalled, // stalled is a detected state, session running doesn't override
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectivePolecatState(tt.item)
			if got != tt.want {
				t.Fatalf("effectivePolecatState() = %q, want %q", got, tt.want)
			}
		})
	}
}

