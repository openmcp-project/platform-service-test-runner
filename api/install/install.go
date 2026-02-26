package install

import (
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	omcpv2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

func InstallOperatorAPIsPlatform(scheme *runtime.Scheme) *runtime.Scheme {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))
	utilruntime.Must(clustersv1alpha1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	return scheme
}

func InstallOperatorAPIsOnboarding(scheme *runtime.Scheme) *runtime.Scheme {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))
	utilruntime.Must(pwv1alpha1.AddToScheme(scheme))
	utilruntime.Must(omcpv2alpha1.AddToScheme(scheme))
	return scheme
}
