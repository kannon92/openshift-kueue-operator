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
	v1beta1 "k8s.io/api/policy/v1beta1"
	gentype "k8s.io/client-go/gentype"
	policyv1beta1 "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
)

// fakeEvictions implements EvictionInterface
type fakeEvictions struct {
	*gentype.FakeClient[*v1beta1.Eviction]
	Fake *FakePolicyV1beta1
}

func newFakeEvictions(fake *FakePolicyV1beta1, namespace string) policyv1beta1.EvictionInterface {
	return &fakeEvictions{
		gentype.NewFakeClient[*v1beta1.Eviction](
			fake.Fake,
			namespace,
			v1beta1.SchemeGroupVersion.WithResource("evictions"),
			v1beta1.SchemeGroupVersion.WithKind("Eviction"),
			func() *v1beta1.Eviction { return &v1beta1.Eviction{} },
		),
		fake,
	}
}