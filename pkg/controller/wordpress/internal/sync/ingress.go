/*
Copyright 2018 Pressinfra SRL.

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

package sync

import (
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/presslabs/controller-util/syncer"

	"github.com/presslabs/wordpress-operator/pkg/cmd/options"
	"github.com/presslabs/wordpress-operator/pkg/internal/wordpress"
)

const ingressClassAnnotationKey = "kubernetes.io/ingress.class"

// NewIngressSyncer returns a new sync.Interface for reconciling web Ingress
func NewIngressSyncer(wp *wordpress.Wordpress, c client.Client, scheme *runtime.Scheme) syncer.Interface {
	objLabels := wp.ComponentLabels(wordpress.WordpressIngress)

	obj := &extv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wp.ComponentName(wordpress.WordpressIngress),
			Namespace: wp.Namespace,
		},
	}

	return syncer.NewObjectSyncer("Ingress", wp.Unwrap(), obj, c, scheme, func(existing runtime.Object) error {
		out := existing.(*extv1beta1.Ingress)
		out.Labels = labels.Merge(labels.Merge(out.Labels, objLabels), controllerLabels)

		if len(out.ObjectMeta.Annotations) == 0 && (len(wp.Spec.IngressAnnotations) > 0 || options.IngressClass != "") {
			out.ObjectMeta.Annotations = make(map[string]string)
		}
		if options.IngressClass != "" {
			out.ObjectMeta.Annotations[ingressClassAnnotationKey] = options.IngressClass
		}
		for k, v := range wp.Spec.IngressAnnotations {
			out.ObjectMeta.Annotations[k] = v
		}

		// if no domains are specified then don't create the ingress
		// if ingress exists then update ingress with no domains
		if len(wp.Spec.Domains) == 0 && out.CreationTimestamp.IsZero() {
			log.Info("no domains are specified - skip creating the ingress", "site", wp.Name, "ingressName", obj.Name)
			return syncer.ErrIgnore
		}

		out.Spec.Rules = composeIngressRules(wp)

		if len(wp.Spec.TLSSecretRef) > 0 {
			tls := extv1beta1.IngressTLS{
				SecretName: string(wp.Spec.TLSSecretRef),
			}
			for _, d := range wp.Spec.Domains {
				tls.Hosts = append(tls.Hosts, string(d))
			}
			out.Spec.TLS = []extv1beta1.IngressTLS{tls}
		} else {
			out.Spec.TLS = nil
		}

		return nil
	})
}

func composeIngressRules(wp *wordpress.Wordpress) []extv1beta1.IngressRule {

	// set domains
	domains := wp.Spec.Domains
	if len(domains) == 0 {
		domains = append(domains, wp.MainDomain())
	}

	bk := extv1beta1.IngressBackend{
		ServiceName: wp.ComponentName(wordpress.WordpressService),
		ServicePort: intstr.FromString("http"),
	}

	bkpaths := []extv1beta1.HTTPIngressPath{{Path: "/", Backend: bk}}

	rules := []extv1beta1.IngressRule{}
	for _, d := range domains {
		rules = append(rules, extv1beta1.IngressRule{
			Host: string(d),
			IngressRuleValue: extv1beta1.IngressRuleValue{
				HTTP: &extv1beta1.HTTPIngressRuleValue{
					Paths: bkpaths,
				},
			},
		})
	}

	return rules
}
