/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "k8s.io/api/core/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	gentype "k8s.io/client-go/gentype"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// fakePersistentVolumeClaims implements PersistentVolumeClaimInterface
type fakePersistentVolumeClaims struct {
	*gentype.FakeClientWithListAndApply[*v1.PersistentVolumeClaim, *v1.PersistentVolumeClaimList, *corev1.PersistentVolumeClaimApplyConfiguration]
	Fake *FakeCoreV1
}

func newFakePersistentVolumeClaims(fake *FakeCoreV1, namespace string) typedcorev1.PersistentVolumeClaimInterface {
	return &fakePersistentVolumeClaims{
		gentype.NewFakeClientWithListAndApply[*v1.PersistentVolumeClaim, *v1.PersistentVolumeClaimList, *corev1.PersistentVolumeClaimApplyConfiguration](
			fake.Fake,
			namespace,
			v1.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
			v1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"),
			func() *v1.PersistentVolumeClaim { return &v1.PersistentVolumeClaim{} },
			func() *v1.PersistentVolumeClaimList { return &v1.PersistentVolumeClaimList{} },
			func(dst, src *v1.PersistentVolumeClaimList) { dst.ListMeta = src.ListMeta },
			func(list *v1.PersistentVolumeClaimList) []*v1.PersistentVolumeClaim {
				return gentype.ToPointerSlice(list.Items)
			},
			func(list *v1.PersistentVolumeClaimList, items []*v1.PersistentVolumeClaim) {
				list.Items = gentype.FromPointerSlice(items)
			},
		),
		fake,
	}
}