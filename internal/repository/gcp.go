package repository

import (
	"context"
	"log"
	"strings"

	billingpb "google.golang.org/genproto/googleapis/cloud/billing/v1"
	containerpb "google.golang.org/genproto/googleapis/container/v1"

	"main/internal/model"
)

type GCPRepository interface {
	GetMachineTypesWithPrices(ctx context.Context) []model.MachineType
	CreateNodePool(ctx context.Context, group model.NodePoolGroup)
}

type gcpRepo struct{}

func NewGCPRepository() GCPRepository {
	return &gcpRepo{}
}

func (g *gcpRepo) GetMachineTypesWithPrices(ctx context.Context) []model.MachineType {
	catalogClient, err := billing.NewCloudCatalogClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create billing client: %v", err)
	}
	defer catalogClient.Close()

	skus := catalogClient.ListSkus(ctx, &billingpb.ListSkusRequest{
		Parent: "services/6F81-5844-456A",
	})

	priceMap := make(map[string]float64)

	for {
		sku, err := skus.Next()
		if err != nil {
			break
		}
		if sku.Category.ResourceFamily != "Compute" || sku.Category.UsageType != "OnDemand" {
			continue
		}
		for _, info := range sku.PricingInfo {
			rate := info.PricingExpression.TieredRates[0].UnitPrice
			price := float64(rate.Nanos)/1e9 + float64(rate.Units)
			priceMap[sku.Description] = price * 730
		}
	}

	types := []model.MachineType{
		{"e2-medium", 1000, 2048, 0},
		{"e2-standard-2", 2000, 4096, 0},
		{"e2-standard-4", 4000, 8192, 0},
	}
	for i := range types {
		t := &types[i]
		for desc, price := range priceMap {
			if strings.Contains(strings.ToLower(desc), t.Type) && strings.Contains(desc, "running") {
				t.Price = price
				break
			}
		}
	}
	return types
}

func (g *gcpRepo) CreateNodePool(ctx context.Context, group model.NodePoolGroup) {
	client, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		log.Printf("GCP client error: %v", err)
		return
	}
	defer client.Close()

	npReq := &containerpb.CreateNodePoolRequest{
		ProjectId: "your-project",
		Zone:      "us-central1-a",
		ClusterId: "your-cluster",
		NodePool: &containerpb.NodePool{
			Name: group.NodeType + "-pool",
			Config: &containerpb.NodeConfig{
				MachineType: group.NodeType,
				Labels:      map[string]string{"node-type": group.NodeType},
			},
			InitialNodeCount: 1,
		},
	}
	_, err = client.CreateNodePool(ctx, npReq)
	if err != nil && !strings.Contains(err.Error(), "alreadyExists") {
		log.Printf("Failed to create node pool: %v", err)
	}
}
