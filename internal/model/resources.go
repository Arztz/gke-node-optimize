package model

import corev1 "k8s.io/api/core/v1"

type PodResources struct {
	Name       string
	Namespace  string
	CPURequest int
	MemRequest int
	CPULimit   int
	MemLimit   int
	Affinity   *corev1.Affinity
}

type NodePoolGroup struct {
	Pods     []PodResources
	TotalCPU int
	TotalRAM int
	NodeType string
	Cost     float64
}

type MachineType struct {
	Type  string
	CPU   int
	RAM   int
	Price float64
}
