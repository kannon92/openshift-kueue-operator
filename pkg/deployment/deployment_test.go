package deployment

import (
    "reflect"
    "testing"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/resource"
)

func TestRemoveResourceLimitsFromDeployment(t *testing.T) {
    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
        Spec: appsv1.DeploymentSpec{
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {
                            Name: "container1",
                            Resources: corev1.ResourceRequirements{
                                Limits: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse("500m"),
                                    corev1.ResourceMemory: resource.MustParse("128Mi"),
                                },
                                Requests: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse("100m"),
                                    corev1.ResourceMemory: resource.MustParse("64Mi"),
                                },
                            },
                        },
                        {
                            Name: "container2",
                            Resources: corev1.ResourceRequirements{
                                Limits: corev1.ResourceList{
                                    corev1.ResourceCPU: resource.MustParse("1"),
                                },
                            },
                        },
                        {
                            Name: "container3",
                            Resources: corev1.ResourceRequirements{
                                Requests: corev1.ResourceList{
                                    corev1.ResourceCPU: resource.MustParse("200m"),
                                },
                            },
                        },
                    },
                },
            },
        },
    }

    updated := RemoveResourceLimitsFromDeployment(dep)

    for i, c := range updated.Spec.Template.Spec.Containers {
        if c.Resources.Limits != nil && len(c.Resources.Limits) != 0 {
            t.Errorf("container %d: expected limits to be empty, got %v", i, c.Resources.Limits)
        }
    }

    // Ensure requests are untouched
    expectedRequests := dep.Spec.Template.Spec.Containers[0].Resources.Requests
    if !reflect.DeepEqual(updated.Spec.Template.Spec.Containers[0].Resources.Requests, expectedRequests) {
        t.Errorf("container 0: requests changed unexpectedly")
    }
}
