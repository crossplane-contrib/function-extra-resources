package main

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/crossplane-contrib/function-extra-resources/input/v1beta1"
)

const (
	// FunctionContextKeyEnvironment is a well-known Context key where the computed Environment
	// will be stored, so that Crossplane v1 and other functions can access it, e.g. function-patch-and-transform.
	FunctionContextKeyEnvironment = "apiextensions.crossplane.io/environment"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

type FetchedResult struct {
	source    v1beta1.ResourceSource
	resources []interface{}
}

// RunFunction runs the Function.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())

	rsp := response.To(req, response.DefaultTTL)

	// Get function input.
	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Errorf("cannot get Function input from %T: %w", req, err))
		return rsp, nil
	}

	// Get XR the pipeline targets.
	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("cannot get observed composite resource: %w", err))
		return rsp, nil
	}

	// Build extraResource Requests.
	requirements, err := buildRequirements(in, oxr)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("could not build extra resource requirements: %w", err))
		return rsp, nil
	}
	rsp.Requirements = requirements

	// The request response cycle for the Crossplane ExtraResources API requires that function-extra-resources
	// tells Crossplane what it wants.
	// Then a new rquest is sent to function-extra-resources with those resources present at the ExtraResources field.
	//
	// function-extra-resources does not know if it has requested the resources already or not.
	//
	// If it has and these resources are now present, proceed with verification and conversion.
	if req.RequiredResources == nil {
		f.log.Debug("No extra resources present, exiting", "requirements", rsp.GetRequirements())
		return rsp, nil
	}

	// Pull extra resources from the ExtraResources request field.
	extraResources, err := request.GetRequiredResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("fetching extra resources %T: %w", req, err))
		return rsp, nil
	}

	// Sort and verify min/max selected.
	// Sorting is required for determinism.
	verifiedExtras, err := verifyAndSortExtras(in, extraResources)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("verifying and sorting extra resources: %w", err))
		return rsp, nil
	}

	var out *unstructured.Unstructured
	var key string

	t := in.Spec.Into.GetIntoType()
	switch t {
	case v1beta1.IntoTypeContext:
		out, err = f.intoContext(verifiedExtras)
		key = in.Spec.Into.GetIntoContextKey()
	case v1beta1.IntoTypeEnvironment:
		out, err = f.intoEnvironment(req, verifiedExtras)
		key = FunctionContextKeyEnvironment
	default:
		err = errors.Errorf("unknown into type: %q", t)
	}

	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}

	s, err := resource.AsStruct(out)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot convert unstructured to protobuf Struct well-known type"))
		return rsp, nil
	}

	response.SetContextKey(rsp, key, structpb.NewStructValue(s))

	return rsp, nil
}

func (f *Function) intoContext(verifiedExtras []FetchedResult) (*unstructured.Unstructured, error) {
	out := &unstructured.Unstructured{Object: map[string]interface{}{}}
	for _, extras := range verifiedExtras {
		if toFieldPath := extras.source.ToFieldPath; toFieldPath != nil && *toFieldPath != "" {
			if err := fieldpath.Pave(out.Object).SetValue(*toFieldPath, extras.resources); err != nil {
				return nil, errors.Wrapf(err, "cannot set nested field path %q", *toFieldPath)
			}
		} else {
			return nil, errors.New("must set toFieldPath for type Context")
		}
	}

	return out, nil
}

func (f *Function) intoEnvironment(req *fnv1.RunFunctionRequest, verifiedExtras []FetchedResult) (*unstructured.Unstructured, error) {
	var inputEnv *unstructured.Unstructured
	if v, ok := request.GetContextKey(req, FunctionContextKeyEnvironment); ok {
		inputEnv = &unstructured.Unstructured{}
		if err := resource.AsObject(v.GetStructValue(), inputEnv); err != nil {
			return nil, errors.Wrapf(err, "cannot get Composition environment from %T context key %q", req, FunctionContextKeyEnvironment)
		}
		f.log.Debug("Loaded Composition environment from Function context", "context-key", FunctionContextKeyEnvironment)
	}

	mergedData := map[string]interface{}{}
	for _, extras := range verifiedExtras {
		for _, extra := range extras.resources {
			if toFieldPath := extras.source.ToFieldPath; toFieldPath != nil && *toFieldPath != "" {
				d := map[string]interface{}{}
				if err := fieldpath.Pave(d).SetValue(*toFieldPath, extra); err != nil {
					return nil, errors.Wrapf(err, "cannot set nested field path %q", *toFieldPath)
				}

				mergedData = mergeMaps(mergedData, d)
			} else if e, ok := extra.(map[string]interface{}); ok {
				mergedData = mergeMaps(mergedData, e)
			} else {
				return nil, errors.New("must set toFieldPath when extracted value is not an object")
			}
		}
	}

	// merge input env if any
	if inputEnv != nil {
		mergedData = mergeMaps(inputEnv.Object, mergedData)
	}

	// build environment and return it in the response as context
	out := &unstructured.Unstructured{Object: mergedData}
	if out.GroupVersionKind().Empty() {
		out.SetGroupVersionKind(schema.GroupVersionKind{Group: "internal.crossplane.io", Kind: "Environment", Version: "v1alpha1"})
	}

	return out, nil
}

// Build requirements takes input and outputs an array of external resoruce requirements to request
// from Crossplane's external resource API.
func buildRequirements(in *v1beta1.Input, xr *resource.Composite) (*fnv1.Requirements, error) { //nolint:gocyclo // Adding non-nil validations increases function complexity.
	extraResources := make(map[string]*fnv1.ResourceSelector, len(in.Spec.ExtraResources))
	for i, extraResource := range in.Spec.ExtraResources {
		extraResName := fmt.Sprintf("resources-%d", i)
		switch extraResource.Type {
		case v1beta1.ResourceSourceTypeReference, "":
			extraResources[extraResName] = &fnv1.ResourceSelector{
				ApiVersion: extraResource.APIVersion,
				Kind:       extraResource.Kind,
				Match: &fnv1.ResourceSelector_MatchName{
					MatchName: extraResource.Ref.Name,
				},
				Namespace: extraResource.Namespace,
			}
		case v1beta1.ResourceSourceTypeSelector:
			matchLabels := map[string]string{}
			for _, selector := range extraResource.Selector.MatchLabels {
				switch selector.GetType() {
				case v1beta1.ResourceSourceSelectorLabelMatcherTypeValue:
					if selector.Value == nil {
						return nil, errors.New("Value cannot be nil for type 'Value'")
					}
					matchLabels[selector.Key] = *selector.Value
				case v1beta1.ResourceSourceSelectorLabelMatcherTypeFromCompositeFieldPath:
					if selector.ValueFromFieldPath == nil {
						return nil, errors.New("ValueFromFieldPath cannot be nil for type 'FromCompositeFieldPath'")
					}
					value, err := fieldpath.Pave(xr.Resource.Object).GetString(*selector.ValueFromFieldPath)
					if err != nil {
						if !selector.FromFieldPathIsOptional() {
							return nil, errors.Wrapf(err, "cannot get value from field path %q", *selector.ValueFromFieldPath)
						}
						continue
					}
					matchLabels[selector.Key] = value
				}
			}
			if len(matchLabels) == 0 {
				continue
			}
			extraResources[extraResName] = &fnv1.ResourceSelector{
				ApiVersion: extraResource.APIVersion,
				Kind:       extraResource.Kind,
				Match: &fnv1.ResourceSelector_MatchLabels{
					MatchLabels: &fnv1.MatchLabels{Labels: matchLabels},
				},
				Namespace: extraResource.Namespace,
			}
		}
	}
	return &fnv1.Requirements{Resources: extraResources}, nil
}

func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = mergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

// Verify Min/Max and sort extra resources by field path within a single kind.
func verifyAndSortExtras(in *v1beta1.Input, extraResources map[string][]resource.Required, //nolint:gocyclo // TODO(reedjosh): refactor
) ([]FetchedResult, error) {
	results := []FetchedResult{}
	for i, extraResource := range in.Spec.ExtraResources {
		extraResName := fmt.Sprintf("resources-%d", i)
		resources, ok := extraResources[extraResName]
		if !ok {
			return nil, errors.Errorf("cannot find expected extra resource %q", extraResName)
		}

		switch extraResource.GetType() {
		case v1beta1.ResourceSourceTypeReference:
			if len(resources) == 0 {
				if in.Spec.Policy.IsResolutionPolicyOptional() {
					continue
				}
				return nil, errors.Errorf("Required extra resource %q not found", extraResName)
			}
			if len(resources) > 1 {
				return nil, errors.Errorf("expected exactly one extra resource %q, got %d", extraResName, len(resources))
			}

		case v1beta1.ResourceSourceTypeSelector:
			selector := extraResource.Selector
			if selector.MinMatch != nil && uint64(len(resources)) < *selector.MinMatch {
				return nil, errors.Errorf("expected at least %d extra resources %q, got %d", *selector.MinMatch, extraResName, len(resources))
			}
			if err := sortExtrasByFieldPath(resources, selector.GetSortByFieldPath()); err != nil {
				return nil, err
			}
			if selector.MaxMatch != nil && uint64(len(resources)) > *selector.MaxMatch {
				resources = resources[:*selector.MaxMatch]
			}
		}

		result := FetchedResult{source: extraResource}
		for _, r := range resources {
			if path := extraResource.FromFieldPath; path != nil {
				if *path == "" {
					return nil, errors.New("fromFieldPath cannot be empty, omit the field to get the whole object")
				}

				// Extract part of the object, from `FromFieldPath`.
				object, err := fieldpath.Pave(r.Resource.Object).GetValue(*path)
				if err != nil {
					return nil, err
				}
				result.resources = append(result.resources, object)
			} else {
				result.resources = append(result.resources, r.Resource.Object)
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// Sort extra resources by field path within a single kind.
func sortExtrasByFieldPath(extras []resource.Required, path string) error { //nolint:gocyclo // TODO(phisco): refactor
	if path == "" {
		return errors.New("cannot sort by empty field path")
	}
	p := make([]struct {
		ec  resource.Required
		val any
	}, len(extras))

	var t reflect.Type
	for i := range extras {
		p[i].ec = extras[i]
		val, err := fieldpath.Pave(extras[i].Resource.Object).GetValue(path)
		if err != nil && !fieldpath.IsNotFound(err) {
			return err
		}
		p[i].val = val
		if val == nil {
			continue
		}
		vt := reflect.TypeOf(val)
		switch {
		case t == nil:
			t = vt
		case t != vt:
			return errors.Errorf("cannot sort values of different types %q and %q", t, vt)
		}
	}
	if t == nil {
		// we either have no values or all values are nil, we can just return
		return nil
	}

	var err error
	sort.Slice(p, func(i, j int) bool {
		vali, valj := p[i].val, p[j].val
		if vali == nil {
			vali = reflect.Zero(t).Interface()
		}
		if valj == nil {
			valj = reflect.Zero(t).Interface()
		}
		switch t.Kind() { //nolint:exhaustive // we only support these types
		case reflect.Float64:
			return vali.(float64) < valj.(float64)
		case reflect.Float32:
			return vali.(float32) < valj.(float32)
		case reflect.Int64:
			return vali.(int64) < valj.(int64)
		case reflect.Int32:
			return vali.(int32) < valj.(int32)
		case reflect.Int16:
			return vali.(int16) < valj.(int16)
		case reflect.Int8:
			return vali.(int8) < valj.(int8)
		case reflect.Int:
			return vali.(int) < valj.(int)
		case reflect.String:
			return vali.(string) < valj.(string)
		default:
			// should never happen
			err = errors.Errorf("unsupported type %q for sorting", t)
			return false
		}
	})
	if err != nil {
		return err
	}

	for i := 0; i < len(extras); i++ {
		extras[i] = p[i].ec
	}
	return nil
}
