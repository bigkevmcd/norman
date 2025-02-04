package lifecycle

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/rancher/norman/objectclient"
	"github.com/rancher/norman/types/slice"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Controllers should check to see if something they want to modify is in a
// namespace that is disallowed.
//
// This is done with a prefix-match so for example gke- will prevent any
// objecting being modified in a namespace beginning with gke-.
var DisallowedNamespaces []string

// IsDisallowedGVK returns true if the resource is a "disallowed
// namespace" i.e. that the namespace should not be written to.
func IsDisallowedNamespace(obj runtime.Object) bool {
	objNS := objectNamespace(obj)
	if objNS == "" {
		return false
	}

	return slices.ContainsFunc(DisallowedNamespaces, func(s string) bool {
		return strings.HasPrefix(objNS, s)
	})
}

// Controllers should check to see if something they want to modify is in this
// list of disallowed GVKs.
var DisallowedGVKs []schema.GroupVersionKind

// IsDisallowedGVK returns true if the resource GVK is listed in the
// disallowed set.
func IsDisallowedGVK(obj runtime.Object) bool {
	objGVK := obj.GetObjectKind().GroupVersionKind()

	return slices.ContainsFunc(DisallowedGVKs, func(gvk schema.GroupVersionKind) bool {
		return objGVK.String() == gvk.String()
	})
}

const (
	created            string = "lifecycle.cattle.io/create"
	finalizerKey       string = "controller.cattle.io/"
	ScopedFinalizerKey string = "clusterscoped.controller.cattle.io/"
)

type ObjectLifecycle interface {
	Create(obj runtime.Object) (runtime.Object, error)
	Finalize(obj runtime.Object) (runtime.Object, error)
	Updated(obj runtime.Object) (runtime.Object, error)
}

type ObjectLifecycleCondition interface {
	HasCreate() bool
	HasFinalize() bool
}

type objectLifecycleAdapter struct {
	name          string
	clusterScoped bool
	lifecycle     ObjectLifecycle
	objectClient  *objectclient.ObjectClient
}

func NewObjectLifecycleAdapter(name string, clusterScoped bool, lifecycle ObjectLifecycle, objectClient *objectclient.ObjectClient) func(key string, obj interface{}) (interface{}, error) {
	o := objectLifecycleAdapter{
		name:          name,
		clusterScoped: clusterScoped,
		lifecycle:     lifecycle,
		objectClient:  objectClient,
	}
	return o.sync
}

func (o *objectLifecycleAdapter) sync(key string, in interface{}) (interface{}, error) {
	if in == nil || reflect.ValueOf(in).IsNil() {
		return nil, nil
	}

	obj, ok := in.(runtime.Object)
	if !ok {
		return nil, nil
	}

	if IsDisallowedNamespace(obj) {
		return obj, nil
	}

	if newObj, cont, err := o.finalize(obj); err != nil || !cont {
		return nil, err
	} else if newObj != nil {
		obj = newObj
	}

	if newObj, cont, err := o.create(obj); err != nil || !cont {
		return nil, err
	} else if newObj != nil {
		obj = newObj
	}

	return o.record(obj, o.lifecycle.Updated)
}

func (o *objectLifecycleAdapter) update(name string, orig, obj runtime.Object) (runtime.Object, error) {
	if obj != nil && orig != nil && !reflect.DeepEqual(orig, obj) {
		newObj, err := o.objectClient.Update(name, obj)
		if newObj != nil {
			return newObj, err
		}
		return obj, err
	}
	if obj == nil {
		return orig, nil
	}
	return obj, nil
}

func (o *objectLifecycleAdapter) finalize(obj runtime.Object) (runtime.Object, bool, error) {
	if !o.hasFinalize() {
		return obj, true, nil
	}

	metadata, err := meta.Accessor(obj)
	if err != nil {
		return obj, false, err
	}

	// Check finalize
	if metadata.GetDeletionTimestamp() == nil {
		return nil, true, nil
	}

	if !slice.ContainsString(metadata.GetFinalizers(), o.constructFinalizerKey()) {
		return nil, false, nil
	}

	newObj, err := o.record(obj, o.lifecycle.Finalize)
	if err != nil {
		return obj, false, err
	}

	obj, err = o.removeFinalizer(o.constructFinalizerKey(), maybeDeepCopy(obj, newObj))
	return obj, false, err
}

func maybeDeepCopy(old, newObj runtime.Object) runtime.Object {
	if old == newObj {
		return old.DeepCopyObject()
	}
	return newObj
}

func (o *objectLifecycleAdapter) removeFinalizer(name string, obj runtime.Object) (runtime.Object, error) {
	for i := 0; i < 3; i++ {
		metadata, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}

		var finalizers []string
		for _, finalizer := range metadata.GetFinalizers() {
			if finalizer == name {
				continue
			}
			finalizers = append(finalizers, finalizer)
		}
		metadata.SetFinalizers(finalizers)

		newObj, err := o.objectClient.Update(metadata.GetName(), obj)
		if err == nil {
			return newObj, nil
		}

		obj, err = o.objectClient.GetNamespaced(metadata.GetNamespace(), metadata.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("failed to remove finalizer on %s", name)
}

func (o *objectLifecycleAdapter) createKey() string {
	return created + "." + o.name
}

func (o *objectLifecycleAdapter) constructFinalizerKey() string {
	if o.clusterScoped {
		return ScopedFinalizerKey + o.name
	}
	return finalizerKey + o.name
}

func (o *objectLifecycleAdapter) hasFinalize() bool {
	cond, ok := o.lifecycle.(ObjectLifecycleCondition)
	return !ok || cond.HasFinalize()
}

func (o *objectLifecycleAdapter) hasCreate() bool {
	cond, ok := o.lifecycle.(ObjectLifecycleCondition)
	return !ok || cond.HasCreate()
}

func (o *objectLifecycleAdapter) record(obj runtime.Object, f func(runtime.Object) (runtime.Object, error)) (runtime.Object, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return obj, err
	}

	origObj := obj
	obj = origObj.DeepCopyObject()
	if newObj, err := checkNil(obj, f); err != nil {
		newObj, _ = o.update(metadata.GetName(), origObj, newObj)
		return newObj, err
	} else if newObj != nil {
		return o.update(metadata.GetName(), origObj, newObj)
	}
	return obj, nil
}

func checkNil(obj runtime.Object, f func(runtime.Object) (runtime.Object, error)) (runtime.Object, error) {
	obj, err := f(obj)
	if obj == nil || reflect.ValueOf(obj).IsNil() {
		return nil, err
	}
	return obj, err
}

func (o *objectLifecycleAdapter) create(obj runtime.Object) (runtime.Object, bool, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return obj, false, err
	}

	if o.isInitialized(metadata) {
		return nil, true, nil
	}

	if o.hasFinalize() {
		obj, err = o.addFinalizer(obj)
		if err != nil {
			return obj, false, err
		}
	}

	if !o.hasCreate() {
		return obj, true, err
	}

	obj, err = o.record(obj, o.lifecycle.Create)
	if err != nil {
		return obj, false, err
	}

	obj, err = o.setInitialized(obj)
	return obj, false, err
}

func (o *objectLifecycleAdapter) isInitialized(metadata metav1.Object) bool {
	initialized := o.createKey()
	return metadata.GetAnnotations()[initialized] == "true"
}

func (o *objectLifecycleAdapter) setInitialized(obj runtime.Object) (runtime.Object, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	initialized := o.createKey()

	if metadata.GetAnnotations() == nil {
		metadata.SetAnnotations(map[string]string{})
	}
	metadata.GetAnnotations()[initialized] = "true"

	updated, err := o.objectClient.Update(metadata.GetName(), obj)
	if err != nil {
		return nil, fmt.Errorf("updating lifecycle annotation %s: %w", initialized, err)
	}

	return updated, err
}

func (o *objectLifecycleAdapter) addFinalizer(obj runtime.Object) (runtime.Object, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	if slice.ContainsString(metadata.GetFinalizers(), o.constructFinalizerKey()) {
		return obj, nil
	}

	obj = obj.DeepCopyObject()
	metadata, err = meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	metadata.SetFinalizers(append(metadata.GetFinalizers(), o.constructFinalizerKey()))
	return o.objectClient.Update(metadata.GetName(), obj)
}

func objectNamespace(obj runtime.Object) string {
	metadata, err := meta.Accessor(obj)
	// This ignores errors...it's not clear how a resource could be passed in
	// that wasn't convertible.
	if err != nil {
		return ""
	}

	if obj.GetObjectKind().GroupVersionKind().Kind == "Namespace" {
		return metadata.GetName()
	}

	return metadata.GetNamespace()
}
