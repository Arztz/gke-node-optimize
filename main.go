package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	container "cloud.google.com/go/container/apiv1"
	"cloud.google.com/go/billing/apiv1"
	billingpb "google.golang.org/genproto/googleapis/cloud/billing/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	containerpb "google.golang.org/genproto/googleapis/container/v1"
	corev1 "k8s.io/api/core/v1"
)

type PodResources struct {
	Name       string
	Namespace  string
	CPURequest int // millicores
	MemRequest int // MB
	CPULimit   int
	MemLimit   int
	Affinity   *corev1.Affinity
}

type NodePoolGroup struct {
	Pods      []PodResources
	TotalCPU  int
	TotalRAM  int
	NodeType  string
	Cost      float64
}

type MachineType struct {
	Type  string
	CPU   int // millicores
	RAM   int // MB
	Price float64 // USD per month
}

var machineTypes = []MachineType{
	{"e2-medium", 1000, 2048, 0},
	{"e2-standard-2", 2000, 4096, 0},
	{"e2-standard-4", 4000, 8192, 0},
}

func main() {
	ctx := context.Background()
	clientset := getKubeClient()

	updatePricesFromGCP(ctx)

	pods := getPodsAndResources(ctx, clientset)
	groups := binPackPods(pods)

	exportCostReport(groups)

	for _, group := range groups {
		createNodePool(ctx, "your-gcp-project", "us-central1-a", "your-cluster-name", group)
		for _, pod := range group.Pods {
			patchPodNodeSelector(clientset, pod.Namespace, pod.Name, group.NodeType, pod.Affinity)
		}
	}
}

func updatePricesFromGCP(ctx context.Context) {
	catalogClient, err := billing.NewCloudCatalogClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create billing catalog client: %v", err)
	}
	defer catalogClient.Close()

	req := &billingpb.ListSkusRequest{
		Parent: "services/6F81-5844-456A", // Compute Engine service ID
		// You can refine the region or SKU types here
	}

	skuIterator := catalogClient.ListSkus(ctx, req)
	skuPrices := map[string]float64{}

	for {
		sku, err := skuIterator.Next()
		if err != nil {
			break
		}
		if sku.Category.ResourceFamily != "Compute" || sku.Category.UsageType != "OnDemand" {
			continue
		}
		for _, pricing := range sku.PricingInfo {
			if pricing.PricingExpression != nil && len(pricing.PricingExpression.TieredRates) > 0 {
				rate := pricing.PricingExpression.TieredRates[0].UnitPrice
				price := float64(rate.Nanos)/1e9 + float64(rate.Units)
				skuPrices[sku.Description] = price
			}
		}
	}

	for i := range machineTypes {
		mt := &machineTypes[i]
		for desc, price := range skuPrices {
			if strings.Contains(strings.ToLower(desc), mt.Type) && strings.Contains(desc, "running") {
				mt.Price = price * 730 // Approx. monthly price (730 hours)
				break
			}
		}
		log.Printf("Updated %s price: $%.2f/month", mt.Type, mt.Price)
	}
}
}

func getKubeClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error creating in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating clientset: %v", err)
	}
	return clientset
}

func getPodsAndResources(ctx context.Context, clientset *kubernetes.Clientset) []PodResources {
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting pods: %v", err)
	}
	var result []PodResources
	for _, pod := range pods.Items {
		cpuReq, memReq, cpuLim, memLim := 0, 0, 0, 0
		for _, c := range pod.Spec.Containers {
			requests := c.Resources.Requests
			limits := c.Resources.Limits
			if cpuQty, ok := requests["cpu"]; ok {
				cpuReq += int(cpuQty.MilliValue())
			}
			if memQty, ok := requests["memory"]; ok {
				memReq += int(memQty.ScaledValue(6))
			}
			if cpuQty, ok := limits["cpu"]; ok {
				cpuLim += int(cpuQty.MilliValue())
			}
			if memQty, ok := limits["memory"]; ok {
				memLim += int(memQty.ScaledValue(6))
			}
		}
		result = append(result, PodResources{
			Name:       pod.Name,
			Namespace:  pod.Namespace,
			CPURequest: cpuReq,
			MemRequest: memReq,
			CPULimit:   cpuLim,
			MemLimit:   memLim,
			Affinity:   pod.Spec.Affinity,
		})
	}
	return result
}

func binPackPods(pods []PodResources) []NodePoolGroup {
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CPURequest > pods[j].CPURequest
	})

	var groups []NodePoolGroup

	for len(pods) > 0 {
		var bestGroup NodePoolGroup
		bestCost := 1e9
		for _, mt := range machineTypes {
			var fit []PodResources
			totalCPU := 0
			totalMem := 0
			for i := 0; i < len(pods); i++ {
				if totalCPU+pods[i].CPURequest <= mt.CPU && totalMem+pods[i].MemRequest <= mt.RAM {
					fit = append(fit, pods[i])
					totalCPU += pods[i].CPURequest
					totalMem += pods[i].MemRequest
				}
			}
			if len(fit) > 0 {
				cost := mt.Price
				if cost < bestCost {
					bestCost = cost
					bestGroup = NodePoolGroup{
						Pods:     fit,
						TotalCPU: totalCPU,
						TotalRAM: totalMem,
						NodeType: mt.Type,
						Cost:     cost,
					}
				}
			}
		}
		if len(bestGroup.Pods) > 0 {
			groups = append(groups, bestGroup)
			var remaining []PodResources
			for _, pod := range pods {
				found := false
				for _, p := range bestGroup.Pods {
					if p.Name == pod.Name && p.Namespace == pod.Namespace {
						found = true
						break
					}
				}
				if !found {
					remaining = append(remaining, pod)
				}
			}
			pods = remaining
		} else {
			break
		}
	}
	return groups
}

func exportCostReport(groups []NodePoolGroup) {
	f, err := os.Create("/tmp/nodepool-cost-report.csv")
	if err != nil {
		log.Printf("Unable to create report file: %v", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "NodeType,TotalCPU(m),TotalRAM(MB),NumPods,Cost(USD)\n")
	for _, g := range groups {
		fmt.Fprintf(f, "%s,%d,%d,%d,%.2f\n", g.NodeType, g.TotalCPU, g.TotalRAM, len(g.Pods), g.Cost)
	}
	log.Println("Exported cost report to /tmp/nodepool-cost-report.csv")
}

func createNodePool(ctx context.Context, projectID, zone, cluster string, group NodePoolGroup) {
	client, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		log.Fatalf("createNodePool: %v", err)
	}
	defer client.Close()

	npReq := &containerpb.CreateNodePoolRequest{
		ProjectId: projectID,
		Zone:      zone,
		ClusterId: cluster,
		NodePool: &containerpb.NodePool{
			Name: group.NodeType + "-pool",
			Config: &containerpb.NodeConfig{
				MachineType: group.NodeType,
				Labels: map[string]string{
					"node-type": group.NodeType,
				},
			},
			InitialNodeCount: 1,
		},
	}
	_, err = client.CreateNodePool(ctx, npReq)
	if err != nil && !strings.Contains(err.Error(), "alreadyExists") {
		log.Printf("Warning: node pool %s creation failed: %v", group.NodeType, err)
	}
}

func patchPodNodeSelector(clientset *kubernetes.Clientset, namespace, podName, nodeType string, affinity *corev1.Affinity) {
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"nodeSelector": map[string]string{
				"node-type": nodeType,
			},
			"affinity": affinity,
		},
	}
	patchBytes, _ := json.Marshal(patch)
	_, err := clientset.CoreV1().Pods(namespace).Patch(
		context.Background(),
		podName,
		"application/merge-patch+json",
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		log.Printf("Warning: patch pod %s/%s failed: %v", namespace, podName, err)
	}
}
