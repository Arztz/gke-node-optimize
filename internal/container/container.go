package container

import (
	"context"

	"go.uber.org/dig"

	"main/internal/controller"
	"main/internal/repository"
)

func BuildContainer(ctx context.Context) *dig.Container {
	c := dig.New()
	_ = c.Provide(func() context.Context { return ctx })
	_ = c.Provide(repository.NewKubeRepository)
	_ = c.Provide(repository.NewGCPRepository)
	_ = c.Provide(controller.NewOptimizerController)
	return c
}
