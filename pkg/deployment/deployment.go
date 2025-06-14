package deployment

import (
    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
)

// RemoveResourceLimitsFromDeployment removes resource limits from all containers in the deployment.
func RemoveResourceLimitsFromDeployment(dep *appsv1.Deployment) *appsv1.Deployment {
    containers := dep.Spec.Template.Spec.Containers
    for i := range containers {
        if containers[i].Resources.Limits != nil {
            containers[i].Resources.Limits = corev1.ResourceList{}
        }
    }
    dep.Spec.Template.Spec.Containers = containers
    return dep
}
