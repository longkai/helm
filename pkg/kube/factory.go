package kube // import "helm.sh/helm/v3/pkg/kube"

import (
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/validation"
)

// Factory provides abstractions that allow the Kubectl command to be extended across multiple types
// of resources and different API sets.
type Factory interface {
	// ToRawKubeConfigLoader return kubeconfig loader as-is
	ToRawKubeConfigLoader() clientcmd.ClientConfig

	// DynamicClient returns a dynamic client ready for use
	DynamicClient() (dynamic.Interface, error)

	// KubernetesClientSet gives you back an external clientset
	KubernetesClientSet() (*kubernetes.Clientset, error)

	// NewBuilder returns an object that assists in loading objects from both disk and the server
	// and which implements the common patterns for CLI interactions with generic resources.
	NewBuilder() *resource.Builder

	// Returns a schema that can validate objects stored on disk.
	Validator(validationDirective string, verifier *resource.QueryParamVerifier) (validation.Schema, error)
	// OpenAPIGetter returns a getter for the openapi schema document
	OpenAPIGetter() discovery.OpenAPISchemaInterface
}
