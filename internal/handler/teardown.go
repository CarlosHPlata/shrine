package handler

import (
	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

func Teardown(b *app.TeardownBundle, team string) error {
	result := planner.PlanTeardown(team, b.Store.Deployments)
	if result.Error != nil {
		return result.Error
	}

	if err := b.Engine.ExecuteTeardown(team, result.Steps); err != nil {
		return err
	}

	return nil
}
