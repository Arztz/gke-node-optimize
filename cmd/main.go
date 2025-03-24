package main

import (
	"context"
	"log"

	"main/internal/container"
)

func main() {
	ctx := context.Background()
	container := container.BuildContainer(ctx)

	err := container.Invoke(func(run func()) {
		run()
	})
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}
}
