package lifecycle

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestIsDisallowedNamespace(t *testing.T) {
	disallowedTests := map[string]struct {
		obj  runtime.Object
		want bool
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
		},
	}

	origNamespaces := DisallowedNamespaces
	t.Cleanup(func() {
		DisallowedNamespaces = origNamespaces
	})
	DisallowedNamespaces = []string{
		"disallowed-ns",
		"disallowed-prefix-",
	}

	for name, tt := range disallowedTests {
		t.Run(name, func(t *testing.T) {
			if v := IsDisallowedNamespace(tt.obj); v != tt.want {
				t.Errorf("IsDisallowedNamespace() got %v, want %v", v, tt.want)
			}
		})
	}
}
