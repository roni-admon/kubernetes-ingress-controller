/*
Copyright 2015 The Kubernetes Authors.

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

package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	testclient "k8s.io/client-go/kubernetes/fake"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kong/kubernetes-ingress-controller/v2/internal/annotations"
)

func TestParseNameNS(t *testing.T) {
	tests := []struct {
		title  string
		input  string
		ns     string
		name   string
		expErr bool
	}{
		{"empty string", "", "", "", true},
		{"demo", "demo", "", "", true},
		{"kube-system", "kube-system", "", "", true},
		{"default/kube-system", "default/kube-system", "default", "kube-system", false},
	}

	for _, test := range tests {
		ns, name, err := ParseNameNS(test.input)
		if test.expErr {
			if err == nil {
				t.Errorf("%v: expected error but returned nil", test.title)
			}
			continue
		}
		if test.ns != ns {
			t.Errorf("%v: expected %v but returned %v", test.title, test.ns, ns)
		}
		if test.name != name {
			t.Errorf("%v: expected %v but returned %v", test.title, test.name, name)
		}
	}
}

func TestGetNodeIP(t *testing.T) {
	ctx := context.Background()

	fKNodes := []struct {
		cs *testclient.Clientset
		n  string
		ea string
	}{
		// empty node list
		{testclient.NewSimpleClientset(), "demo", ""},

		// node not exist
		{testclient.NewSimpleClientset(&corev1.NodeList{Items: []corev1.Node{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.1",
					},
				},
			},
		}}}), "notexistnode", ""},

		// node  exist
		{testclient.NewSimpleClientset(&corev1.NodeList{Items: []corev1.Node{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.1",
					},
				},
			},
		}}}), "demo", "10.0.0.1"},

		// search the correct node
		{testclient.NewSimpleClientset(&corev1.NodeList{Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "demo1",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "10.0.0.1",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "demo2",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "10.0.0.2",
						},
					},
				},
			},
		}}), "demo2", "10.0.0.2"},

		// get NodeExternalIP
		{testclient.NewSimpleClientset(&corev1.NodeList{Items: []corev1.Node{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.1",
					}, {
						Type:    corev1.NodeExternalIP,
						Address: "10.0.0.2",
					},
				},
			},
		}}}), "demo", "10.0.0.2"},

		// get NodeInternalIP
		{testclient.NewSimpleClientset(&corev1.NodeList{Items: []corev1.Node{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeExternalIP,
						Address: "",
					}, {
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.2",
					},
				},
			},
		}}}), "demo", "10.0.0.2"},
	}

	for _, fk := range fKNodes {
		address := GetNodeIPOrName(ctx, fk.cs, fk.n)
		if address != fk.ea {
			t.Errorf("expected %s, but returned %s", fk.ea, address)
		}
	}
}

func TestGetPodDetails(t *testing.T) {
	ctx := context.Background()
	// POD_NAME & POD_NAMESPACE not exist
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")
	_, err1 := GetPodDetails(ctx, testclient.NewSimpleClientset())
	if err1 == nil {
		t.Errorf("expected an error but returned nil")
	}

	// POD_NAME not exist
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", corev1.NamespaceDefault)
	_, err2 := GetPodDetails(ctx, testclient.NewSimpleClientset())
	if err2 == nil {
		t.Errorf("expected an error but returned nil")
	}

	// POD_NAMESPACE not exist
	t.Setenv("POD_NAME", "testpod")
	t.Setenv("POD_NAMESPACE", "")
	_, err3 := GetPodDetails(ctx, testclient.NewSimpleClientset())
	if err3 == nil {
		t.Errorf("expected an error but returned nil")
	}

	// POD not exist
	t.Setenv("POD_NAME", "testpod")
	t.Setenv("POD_NAMESPACE", corev1.NamespaceDefault)
	_, err4 := GetPodDetails(ctx, testclient.NewSimpleClientset())
	if err4 == nil {
		t.Errorf("expected an error but returned nil")
	}

	// success to get PodInfo
	fkClient := testclient.NewSimpleClientset(
		&corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpod",
				Namespace: corev1.NamespaceDefault,
				Labels: map[string]string{
					"first":  "first_label",
					"second": "second_label",
				},
			},
		}}},
		&corev1.NodeList{Items: []corev1.Node{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.1",
					},
				},
			},
		}}})

	epi, err5 := GetPodDetails(ctx, fkClient)
	if err5 != nil {
		t.Errorf("expected a PodInfo but returned error")
		return
	}

	if epi == nil {
		t.Errorf("expected a PodInfo but returned nil")
	}
}

func TestGenerateTagsForObject(t *testing.T) {
	actualTagSet := map[string]bool{}
	expectedTagSet := map[string]bool{
		K8sNamespaceTagPrefix + "aitmatov": true,
		K8sNameTagPrefix + "yedigei":       true,
		K8sUIDTagPrefix + "buryani":        true,
		K8sKindTagPrefix + "adam":          true,
		K8sGroupTagPrefix + "sary.ozek":    true,
		K8sVersionTagPrefix + "v1beta100":  true,
		"snaryad-soqqısı":                  true,
		"temir-jol":                        true,
	}

	// somewhat unintuitively, declaring a static HTTPRoute does not inherently give it HTTPRoute TypeMeta
	// to deal with this, the test manually sets fake values, allowing the tag generator to run as if it
	// had an object with actual meta, like you'd get if you used the API server get functions.
	tmeta := metav1.TypeMeta{}
	tmeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "sary.ozek",
		Version: "v1beta100",
		Kind:    "adam",
	})
	testObj := &gatewayv1beta1.HTTPRoute{
		TypeMeta: tmeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      "yedigei",
			Namespace: "aitmatov",
			UID:       "buryani",
			Annotations: map[string]string{
				annotations.AnnotationPrefix + annotations.UserTagKey: "snaryad-soqqısı,temir-jol",
			},
		},
	}

	tags := GenerateTagsForObject(testObj)
	for _, tag := range tags {
		actualTagSet[*tag] = true
	}

	for e := range expectedTagSet {
		_, ok := actualTagSet[e]
		assert.Truef(t, ok, "expected %s tag not present", e)
	}

	for a := range actualTagSet {
		_, ok := expectedTagSet[a]
		assert.Truef(t, ok, "unexpected %s tag present", a)
	}
}
