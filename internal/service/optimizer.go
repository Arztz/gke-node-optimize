package service

import (
	"sort"

	"gke-optimizer/internal/model"
)

func BinPackPods(pods []model.PodResources, machines []model.MachineType) []model.NodePoolGroup {
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CPURequest > pods[j].CPURequest
	})

	var groups []model.NodePoolGroup

	for len(pods) > 0 {
		var bestGroup model.NodePoolGroup
		bestCost := 1e9
		for _, mt := range machines {
			var fit []model.PodResources
			totalCPU := 0
			totalMem := 0
			for _, pod := range pods {
				if totalCPU+pod.CPURequest <= mt.CPU && totalMem+pod.MemRequest <= mt.RAM {
					fit = append(fit, pod)
					totalCPU += pod.CPURequest
					totalMem += pod.MemRequest
				}
			}
			if len(fit) > 0 && mt.Price < bestCost {
				bestGroup = model.NodePoolGroup{
					Pods:     fit,
					TotalCPU: totalCPU,
					TotalRAM: totalMem,
					NodeType: mt.Type,
					Cost:     mt.Price,
				}
				bestCost = mt.Price
			}
		}
		if len(bestGroup.Pods) == 0 {
			break
		}
		groups = append(groups, bestGroup)

		var remaining []model.PodResources
		for _, pod := range pods {
			found := false
			for _, used := range bestGroup.Pods {
				if pod.Name == used.Name && pod.Namespace == used.Namespace {
					found = true
					break
				}
			}
			if !found {
				remaining = append(remaining, pod)
			}
		}
		pods = remaining
	}
	return groups
}
