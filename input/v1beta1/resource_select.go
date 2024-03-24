package v1beta1

/*
Copyright 2022 The Crossplane Authors.

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

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// An InputSpec specifies extra resource(s) for rendering composed resources.
type InputSpec struct {
	// ExtraResources selects a list of `ExtraResource`s. The resolved
	// resources are stored in the composite resource at
	// `spec.extraResourceRefs` and is only updated if it is null.
	ExtraResources []ResourceSource `json:"extraResources"`

	// Policy represents the Resolve and Resolution policies which apply to
	// all ResourceSourceReferences in ExtraResources list.
	// +optional
	Policy *xpv1.Policy `json:"policy,omitempty"`
}

// Policy represents the Resolve and Resolution policies of Reference instance.
type Policy struct {
	// Resolution specifies whether resolution of this reference is required.
	// The default is 'Required', which means the reconcile will fail if the
	// reference cannot be resolved. 'Optional' means this reference will be
	// a no-op if it cannot be resolved.
	// +optional
	// +kubebuilder:default=Required
	// +kubebuilder:validation:Enum=Required;Optional
	Resolution *xpv1.ResolutionPolicy `json:"resolution,omitempty"`
}

// ResourceSourceType specifies the way the ExtraResource is selected.
type ResourceSourceType string

const (
	// ResourceSourceTypeReference by name.
	ResourceSourceTypeReference ResourceSourceType = "Reference"
	// ResourceSourceTypeSelector by labels.
	ResourceSourceTypeSelector ResourceSourceType = "Selector"
)

// ResourceSource selects a ExtraResource.
type ResourceSource struct {
	// Type specifies the way the ExtraResource is selected.
	// Default is `Reference`
	// +optional
	// +kubebuilder:validation:Enum=Reference;Selector
	// +kubebuilder:default=Reference
	Type ResourceSourceType `json:"type,omitempty"`

	// Ref is a named reference to a single ExtraResource.
	// Either Ref or Selector is required.
	// +optional
	Ref *ResourceSourceReference `json:"ref,omitempty"`

	// Selector selects ExtraResource(s) via labels.
	// +optional
	Selector *ResourceSourceSelector `json:"selector,omitempty"`

	// Kind is the kubernetes kind of the target extra resource(s).
	Kind string `json:"kind,omitempty"`

	// APIVersion is the kubernetes API Version of the target extra resource(s).
	APIVersion string `json:"apiVersion,omitempty"`

	// Into is the key into which extra resources for this selector will be placed.
	Into string `json:"into"`
}

// GetType returns the type of the resource source, returning the default if not set.
func (e *ResourceSource) GetType() ResourceSourceType {
	if e == nil || e.Type == "" {
		return ResourceSourceTypeReference
	}
	return e.Type
}

// An ResourceSourceReference references an ExtraResource by it's name.
type ResourceSourceReference struct {
	// The name of the object.
	Name string `json:"name"`
}

// An ResourceSourceSelector selects an ExtraResource via labels.
type ResourceSourceSelector struct {
	// MaxMatch specifies the number of extracted ExtraResources in Multiple mode, extracts all if nil.
	MaxMatch *uint64 `json:"maxMatch,omitempty"`

	// MinMatch specifies the required minimum of extracted ExtraResources in Multiple mode.
	MinMatch *uint64 `json:"minMatch,omitempty"`

	// SortByFieldPath is the path to the field based on which list of ExtraResources is alphabetically sorted.
	// +kubebuilder:default="metadata.name"
	SortByFieldPath string `json:"sortByFieldPath,omitempty"`

	// MatchLabels ensures an object with matching labels is selected.
	MatchLabels []ResourceSourceSelectorLabelMatcher `json:"matchLabels,omitempty"`
}

// GetSortByFieldPath returns the sort by path if set or a sane default.
func (e *ResourceSourceSelector) GetSortByFieldPath() string {
	if e == nil || e.SortByFieldPath == "" {
		return "metadata.name"
	}
	return e.SortByFieldPath
}

// ResourceSourceSelectorLabelMatcherType specifies where the value for a label comes from.
type ResourceSourceSelectorLabelMatcherType string

const (
	// ResourceSourceSelectorLabelMatcherTypeFromCompositeFieldPath extracts
	// the label value from a composite fieldpath.
	ResourceSourceSelectorLabelMatcherTypeFromCompositeFieldPath ResourceSourceSelectorLabelMatcherType = "FromCompositeFieldPath"
	// ResourceSourceSelectorLabelMatcherTypeValue uses a literal as label
	// value.
	ResourceSourceSelectorLabelMatcherTypeValue ResourceSourceSelectorLabelMatcherType = "Value"
)

// An ResourceSourceSelectorLabelMatcher acts like a k8s label selector but
// can draw the label value from a different path.
type ResourceSourceSelectorLabelMatcher struct {
	// Type specifies where the value for a label comes from.
	// +optional
	// +kubebuilder:validation:Enum=FromCompositeFieldPath;Value
	// +kubebuilder:default=FromCompositeFieldPath
	Type ResourceSourceSelectorLabelMatcherType `json:"type,omitempty"`

	// Key of the label to match.
	Key string `json:"key"`

	// ValueFromFieldPath specifies the field path to look for the label value.
	ValueFromFieldPath *string `json:"valueFromFieldPath,omitempty"`

	// FromFieldPathPolicy specifies the policy for the valueFromFieldPath.
	// The default is Required, meaning that an error will be returned if the
	// field is not found in the composite resource.
	// Optional means that if the field is not found in the composite resource,
	// that label pair will just be skipped. N.B. other specified label
	// matchers will still be used to retrieve the desired
	// resource config, if any.
	// +kubebuilder:validation:Enum=Optional;Required
	// +kubebuilder:default=Required
	FromFieldPathPolicy *FromFieldPathPolicy `json:"fromFieldPathPolicy,omitempty"`

	// Value specifies a literal label value.
	Value *string `json:"value,omitempty"`
}

// FromFieldPathIsOptional returns true if the FromFieldPathPolicy is set to
// +optional
func (e *ResourceSourceSelectorLabelMatcher) FromFieldPathIsOptional() bool {
	return e.FromFieldPathPolicy != nil && *e.FromFieldPathPolicy == FromFieldPathPolicyOptional
}

// GetType returns the type of the label matcher, returning the default if not set.
func (e *ResourceSourceSelectorLabelMatcher) GetType() ResourceSourceSelectorLabelMatcherType {
	if e == nil || e.Type == "" {
		return ResourceSourceSelectorLabelMatcherTypeFromCompositeFieldPath
	}
	return e.Type
}

// A FromFieldPathPolicy determines how to patch from a field path.
type FromFieldPathPolicy string

// FromFieldPath patch policies.
const (
	FromFieldPathPolicyOptional FromFieldPathPolicy = "Optional"
	FromFieldPathPolicyRequired FromFieldPathPolicy = "Required"
)

// A PatchPolicy configures the specifics of patching behaviour.
type PatchPolicy struct {
	// FromFieldPath specifies how to patch from a field path. The default is
	// 'Optional', which means the patch will be a no-op if the specified
	// fromFieldPath does not exist. Use 'Required' if the patch should fail if
	// the specified path does not exist.
	// +kubebuilder:validation:Enum=Optional;Required
	// +optional
	FromFieldPath *FromFieldPathPolicy `json:"fromFieldPath,omitempty"`
	MergeOptions  *xpv1.MergeOptions   `json:"mergeOptions,omitempty"`
}

// GetFromFieldPathPolicy returns the FromFieldPathPolicy for this PatchPolicy, defaulting to FromFieldPathPolicyOptional if not specified.
func (pp *PatchPolicy) GetFromFieldPathPolicy() FromFieldPathPolicy {
	if pp == nil || pp.FromFieldPath == nil {
		return FromFieldPathPolicyOptional
	}
	return *pp.FromFieldPath
}
