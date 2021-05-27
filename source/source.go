/*
Copyright 2017 The Kubernetes Authors.

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

package source

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/source/annotations"
)

const (
	controllerAnnotationKey       = annotations.ControllerKey
	hostnameAnnotationKey         = annotations.HostnameKey
	accessAnnotationKey           = annotations.AccessKey
	endpointsTypeAnnotationKey    = annotations.EndpointsTypeKey
	targetAnnotationKey           = annotations.TargetKey
	ttlAnnotationKey              = annotations.TtlKey
	aliasAnnotationKey            = annotations.AliasKey
	ingressHostnameSourceKey      = annotations.IngressHostnameSourceKey
	controllerAnnotationValue     = annotations.ControllerValue
	internalHostnameAnnotationKey = annotations.InternalHostnameKey

	EndpointsTypeNodeExternalIP = "NodeExternalIP"
	EndpointsTypeHostIP         = "HostIP"
)

// Provider-specific annotations
const (
	// The annotation used for determining if traffic will go through Cloudflare
	CloudflareProxiedKey = "external-dns.alpha.kubernetes.io/cloudflare-proxied"

	SetIdentifierKey = "external-dns.alpha.kubernetes.io/set-identifier"

	// This annotation is used to distinguish NodePort services that will create load balancers
	// via aws-load-balancer-controller-v2
	AwsLoadBalancerTypeAnnotation = "service.beta.kubernetes.io/aws-load-balancer-type"
)

const (
	ttlMinimum = 1
	ttlMaximum = math.MaxInt32
)

// Source defines the interface Endpoint sources should implement.
type Source interface {
	Endpoints(ctx context.Context) ([]*endpoint.Endpoint, error)
	// AddEventHandler adds an event handler that should be triggered if something in source changes
	AddEventHandler(context.Context, func())
}

type kubeObject interface {
	runtime.Object
	metav1.Object
}

func getAccessFromAnnotations(input map[string]string) string {
	return input[accessAnnotationKey]
}

func getEndpointsTypeFromAnnotations(annotations map[string]string) string {
	return annotations[endpointsTypeAnnotationKey]
}

func getInternalHostnamesFromAnnotations(annotations map[string]string) []string {
	internalHostnameAnnotation, exists := annotations[internalHostnameAnnotationKey]
	if !exists {
		return nil
	}
	return splitHostnameAnnotation(internalHostnameAnnotation)
}

func splitHostnameAnnotation(annotation string) []string {
	return strings.Split(strings.Replace(annotation, " ", "", -1), ",")
}

func getAliasFromAnnotations(annotations map[string]string) bool {
	aliasAnnotation, exists := annotations[aliasAnnotationKey]
	return exists && aliasAnnotation == "true"
}

func getProviderSpecificAnnotations(annotations map[string]string) (endpoint.ProviderSpecific, string) {
	providerSpecificAnnotations := endpoint.ProviderSpecific{}

	v, exists := annotations[CloudflareProxiedKey]
	if exists {
		providerSpecificAnnotations = append(providerSpecificAnnotations, endpoint.ProviderSpecificProperty{
			Name:  CloudflareProxiedKey,
			Value: v,
		})
	}

	// aws-load-balancer-v2 NodePort Service Annotation
	if v, exists := annotations[AwsLoadBalancerTypeAnnotation]; exists {
		providerSpecificAnnotations = append(providerSpecificAnnotations, endpoint.ProviderSpecificProperty{
			Name:  AwsLoadBalancerTypeAnnotation,
			Value: v,
		})
	}

	if getAliasFromAnnotations(annotations) {
		providerSpecificAnnotations = append(providerSpecificAnnotations, endpoint.ProviderSpecificProperty{
			Name:  "alias",
			Value: "true",
		})
	}
	setIdentifier := ""
	for k, v := range annotations {
		if k == SetIdentifierKey {
			setIdentifier = v
		} else if strings.HasPrefix(k, "external-dns.alpha.kubernetes.io/aws-") {
			attr := strings.TrimPrefix(k, "external-dns.alpha.kubernetes.io/aws-")
			providerSpecificAnnotations = append(providerSpecificAnnotations, endpoint.ProviderSpecificProperty{
				Name:  fmt.Sprintf("aws/%s", attr),
				Value: v,
			})
		} else if strings.HasPrefix(k, "external-dns.alpha.kubernetes.io/scw-") {
			attr := strings.TrimPrefix(k, "external-dns.alpha.kubernetes.io/scw-")
			providerSpecificAnnotations = append(providerSpecificAnnotations, endpoint.ProviderSpecificProperty{
				Name:  fmt.Sprintf("scw/%s", attr),
				Value: v,
			})
		} else if strings.HasPrefix(k, "external-dns.alpha.kubernetes.io/ibmcloud-") {
			attr := strings.TrimPrefix(k, "external-dns.alpha.kubernetes.io/ibmcloud-")
			providerSpecificAnnotations = append(providerSpecificAnnotations, endpoint.ProviderSpecificProperty{
				Name:  fmt.Sprintf("ibmcloud-%s", attr),
				Value: v,
			})
		}
	}
	return providerSpecificAnnotations, setIdentifier
}

// getTargetsFromTargetAnnotation gets endpoints from optional "target" annotation.
// Returns empty endpoints array if none are found.
func getTargetsFromTargetAnnotation(annotations map[string]string) endpoint.Targets {
	var targets endpoint.Targets

	// Get the desired hostname of the ingress from the annotation.
	targetAnnotation, exists := annotations[targetAnnotationKey]
	if exists && targetAnnotation != "" {
		// splits the hostname annotation and removes the trailing periods
		targetsList := strings.Split(strings.Replace(targetAnnotation, " ", "", -1), ",")
		for _, targetHostname := range targetsList {
			targetHostname = strings.TrimSuffix(targetHostname, ".")
			targets = append(targets, targetHostname)
		}
	}
	return targets
}

// suitableType returns the DNS resource record type suitable for the target.
// In this case type A for IPs and type CNAME for everything else.
func suitableType(target string) string {
	if net.ParseIP(target) != nil && net.ParseIP(target).To4() != nil {
		return endpoint.RecordTypeA
	} else if net.ParseIP(target) != nil && net.ParseIP(target).To16() != nil {
		return endpoint.RecordTypeAAAA
	}
	return endpoint.RecordTypeCNAME
}

// endpointsForHostname returns the endpoint objects for each host-target combination.
func endpointsForHostname(hostname string, targets endpoint.Targets, ttl endpoint.TTL, providerSpecific endpoint.ProviderSpecific, setIdentifier string) []*endpoint.Endpoint {
	var endpoints []*endpoint.Endpoint

	var aTargets endpoint.Targets
	var aaaaTargets endpoint.Targets
	var cnameTargets endpoint.Targets

	for _, t := range targets {
		switch suitableType(t) {
		case endpoint.RecordTypeA:
			if isIPv6String(t) {
				continue
			}
			aTargets = append(aTargets, t)
		case endpoint.RecordTypeAAAA:
			if !isIPv6String(t) {
				continue
			}
			aaaaTargets = append(aaaaTargets, t)
		default:
			cnameTargets = append(cnameTargets, t)
		}
	}

	if len(aTargets) > 0 {
		epA := &endpoint.Endpoint{
			DNSName:          strings.TrimSuffix(hostname, "."),
			Targets:          aTargets,
			RecordTTL:        ttl,
			RecordType:       endpoint.RecordTypeA,
			Labels:           endpoint.NewLabels(),
			ProviderSpecific: providerSpecific,
			SetIdentifier:    setIdentifier,
		}
		endpoints = append(endpoints, epA)
	}

	if len(aaaaTargets) > 0 {
		epAAAA := &endpoint.Endpoint{
			DNSName:          strings.TrimSuffix(hostname, "."),
			Targets:          aaaaTargets,
			RecordTTL:        ttl,
			RecordType:       endpoint.RecordTypeAAAA,
			Labels:           endpoint.NewLabels(),
			ProviderSpecific: providerSpecific,
			SetIdentifier:    setIdentifier,
		}
		endpoints = append(endpoints, epAAAA)
	}

	if len(cnameTargets) > 0 {
		epCNAME := &endpoint.Endpoint{
			DNSName:          strings.TrimSuffix(hostname, "."),
			Targets:          cnameTargets,
			RecordTTL:        ttl,
			RecordType:       endpoint.RecordTypeCNAME,
			Labels:           endpoint.NewLabels(),
			ProviderSpecific: providerSpecific,
			SetIdentifier:    setIdentifier,
		}
		endpoints = append(endpoints, epCNAME)
	}
	return endpoints
}

func getLabelSelector(annotationFilter string) (labels.Selector, error) {
	labelSelector, err := metav1.ParseToLabelSelector(annotationFilter)
	if err != nil {
		return nil, err
	}
	return metav1.LabelSelectorAsSelector(labelSelector)
}

func matchLabelSelector(selector labels.Selector, srcAnnotations map[string]string) bool {
	return selector.Matches(labels.Set(srcAnnotations))
}

type eventHandlerFunc func()

func (fn eventHandlerFunc) OnAdd(obj interface{}, isInInitialList bool) { fn() }
func (fn eventHandlerFunc) OnUpdate(oldObj, newObj interface{})         { fn() }
func (fn eventHandlerFunc) OnDelete(obj interface{})                    { fn() }
