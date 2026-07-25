package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"open-monitoring/internal/stress"
)

// The stress panel is the other half of a monitoring app: the charts only mean
// something once the machine has been pushed somewhere it does not normally
// go. The runner does the work; this file is the binding surface, and the
// "stress" event mirrors the "sample" one — a snapshot pushed every second for
// as long as anything is running.

// StartStress begins loading the requested subsystems. Targets already under
// load keep running with their original deadline instead of restarting, so
// adding a second one mid-run does not reset the first.
func (a *App) StartStress(opts stress.Options) (stress.Status, error) {
	return a.stress.Start(opts)
}

// StopStress ends the named jobs, or every running job when the list is empty.
func (a *App) StopStress(targets []string) stress.Status {
	return a.stress.Stop(targets)
}

func (a *App) GetStressStatus() stress.Status {
	return a.stress.Status()
}

// GetStressDefaults gives the panel its starting values, including the thread
// count and temporary directory of this machine rather than the UI's guesses.
func (a *App) GetStressDefaults() stress.Options {
	return stress.Defaults()
}

// emitStress is the runner's callback; it is called once a second while a job
// is alive, and once more as the last one finishes.
func (a *App) emitStress(s stress.Status) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "stress", s)
	}
}
