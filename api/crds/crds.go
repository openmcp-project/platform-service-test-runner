package crds

import (
	"embed"

	crdutil "github.com/openmcp-project/controller-utils/pkg/crds"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// CRDFS embeds the CRD manifest files.
//
//go:embed manifests/*.yaml
var CRDFS embed.FS

// CRDs returns all CustomResourceDefinitions from the embedded manifests.
func CRDs() ([]*apiextv1.CustomResourceDefinition, error) {
	return crdutil.CRDsFromFileSystem(CRDFS, "manifests")
}
