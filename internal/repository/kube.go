package repository

import (
	"context"
	"encoding/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"main/internal/model"
)

type KubeRepository interface {
	GetPodsAndResources(ctx context.Context) []model.PodResources
	PatchPodNodeSelector(ctx context.Context, pod model.PodResources, nodeType string)
}

type kubeRepo struct {
	client *kubernetes.Clientset
}

func NewKubeRepository() KubeRepository {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to load in-cluster config: %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to create k8s client: %v", err)
	}
	return &kubeRepo{client: client}
}

func (k *kubeRepo) GetPodsAndResources(ctx context.Context) []model.PodResources {
	pods, err := k.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("List pods error: %v", err)
	}
	var result []model.PodResources
	for _, pod := range pods.Items {
		cpu, mem := 0, 0
		for _, c := range pod.Spec.Containers {
			cpu += int(c.Resources.Requests.Cpu().MilliValue())
			mem += int(c.Resources.Requests.Memory().ScaledValue(6))
		}
		result = append(result, model.PodResources{
			Name: pod.Name, Namespace: pod.Namespace,
			CPURequest: cpu, MemRequest: mem,
			Affinity: pod.Spec.Affinity,
		})
	}
	return result
}

func (k *kubeRepo) PatchPodNodeSelector(ctx context.Context, pod model.PodResources, nodeType string) {
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"nodeSelector": map[string]string{
				"node-type": nodeType,
			},
			"affinity": pod.Affinity,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		log.Printf("Failed to marshal patch for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return
	}

	patchType := types.MergePatchType
	_, err = k.client.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, patchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		log.Printf("Failed to patch pod %s/%s: %v", pod.Namespace, pod.Name, err)
	} else {
		log.Printf("✅ Patched pod %s/%s with nodeSelector=node-type=%s", pod.Namespace, pod.Name, nodeType)
	}
}
