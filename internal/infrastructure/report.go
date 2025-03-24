package infrastructure

import (
	"fmt"
	"log"
	"os"

	"main/internal/model"
)

func ExportCostReport(groups []model.NodePoolGroup) {
	f, err := os.Create("/tmp/nodepool-cost-report.csv")
	if err != nil {
		log.Printf("Failed to write report: %v", err)
		return
	}
	defer f.Close()

	fmt.Fprintln(f, "NodeType,TotalCPU(m),TotalRAM(MB),NumPods,Cost(USD)")
	for _, g := range groups {
		fmt.Fprintf(f, "%s,%d,%d,%d,%.2f\n", g.NodeType, g.TotalCPU, g.TotalRAM, len(g.Pods), g.Cost)
	}
	log.Println("📄 Exported report to /tmp/nodepool-cost-report.csv")
}
