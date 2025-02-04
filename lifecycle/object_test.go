package lifecycle

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsDisallowedNamespace(t *testing.T) {
	disallowedTests := map[string]struct {
		disallowedNamespaces []string
		obj                  runtime.Object
		want                 bool
	}{
		"resource is a disallowed namespace": {
			obj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "disallowed-ns",
				},
			},
			want: true,
			disallowedNamespaces: []string{
				"disallowed-ns",
				"disallowed-prefix-",
			},
		},
		"resource is not a disallowed namespace": {
			obj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "allowed-ns",
				},
			},
			want: false,
			disallowedNamespaces: []string{
				"disallowed-ns",
				"disallowed-prefix-",
			},
		},
		"resource is a disallowed prefix namespace": {
			obj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "disallowed-prefix-",
				},
			},
			want: true,
			disallowedNamespaces: []string{
				"disallowed-ns",
				"disallowed-prefix-",
			},
		},
		"resource is a resource in a disallowed namespace": {
			obj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "disallowed-ns",
				},
			},
			want: true,
			disallowedNamespaces: []string{
				"disallowed-ns",
				"disallowed-prefix-",
			},
		},
		"resource is a resource in an allowed namespace": {
			obj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "allowed-ns",
				},
			},
			want: false,
			disallowedNamespaces: []string{
				"disallowed-ns",
				"disallowed-prefix-",
			},
		},
		"resource when no namespaces are disallowed": {
			obj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "allowed-ns",
				},
			},
			want: false,
		},
		"namespace when no namespaces are disallowed": {
			obj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			},
			want: false,
		},
	}

	for name, tt := range disallowedTests {
		t.Run(name, func(t *testing.T) {
			if tt.disallowedNamespaces != nil {
				origNamespaces := DisallowedNamespaces
				t.Cleanup(func() {
					DisallowedNamespaces = origNamespaces
				})
				DisallowedNamespaces = tt.disallowedNamespaces
			}
			if v := IsDisallowedNamespace(tt.obj); v != tt.want {
				t.Errorf("IsDisallowedNamespace() got %v, want %v", v, tt.want)
			}
		})
	}
}

func TestIsDisallowedResource(t *testing.T) {
	disallowedTests := map[string]struct {
		obj  runtime.Object
		want bool
	}{
		"resource is a disallowed resource": {
			obj: &corev1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "localhost.localdomain",
				},
			},
			want: true,
		},
		"resource is not a disallowed resource": {
			obj: &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "allowed-ns",
				},
			},
			want: false,
		},
	}

	origKinds := DisallowedGVKs
	t.Cleanup(func() {
		DisallowedGVKs = origKinds
	})
	DisallowedGVKs = []schema.GroupVersionKind{
		{
			Group:   "",
			Version: "v1",
			Kind:    "Node",
		},
	}
	for name, tt := range disallowedTests {
		t.Run(name, func(t *testing.T) {
			if v := IsDisallowedGVK(tt.obj); v != tt.want {
				t.Errorf("IsDisallowedGVK() got %v, want %v", v, tt.want)
			}
		})
	}
}
