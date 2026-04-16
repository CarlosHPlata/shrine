package planner

import "github.com/CarlosHPlata/shrine/internal/state"

type PlanResult struct {
	Steps         []PlannedStep
	Error         error
	ValidationErr []error
}

func Plan(dir string, store state.Store) PlanResult {
	set, err := LoadDir(dir)
	if err != nil {
		return PlanResult{Error: err}
	}

	if errs := Resolve(set, store); len(errs) > 0 {
		return PlanResult{ValidationErr: errs}
	}

	return PlanResult{Steps: Order(set)}
}
