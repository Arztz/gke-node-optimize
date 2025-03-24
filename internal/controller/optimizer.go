package controller

import (
	"context"
	"log"

	"main/internal/infrastructure"
	"main/internal/repository"
	"main/internal/service"
)

type OptimizerController struct {
	Run func()
}

func NewOptimizerController(
	ctx context.Context,
	kube repository.KubeRepository,
	gcp repository.GCPRepository,
) func() {
	return func() {
		pods := kube.GetPodsAndResources(ctx)
		machines := gcp.GetMachineTypesWithPrices(ctx)
		groups := service.BinPackPods(pods, machines)

		infrastructure.ExportCostReport(groups)

		for _, group := range groups {
			gcp.CreateNodePool(ctx, group)
			for _, pod := range group.Pods {
				kube.PatchPodNodeSelector(ctx, pod, group.NodeType)
			}
		}
		log.Println("✅ Optimization complete")
	}
}
