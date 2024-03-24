package main

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/function-extra-resources/input/v1beta1"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
)

// Key to retrieve extras at.
const (
	FunctionContextKeyExtraResources = "apiextensions.crossplane.io/extra-resources"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction runs the Function.
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) {
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
	if req.ExtraResources == nil {
		f.log.Debug("No extra resources present, exiting", "requirements", rsp.GetRequirements())
		return rsp, nil
	}

	// Pull extra resources from the ExtraResources request field.
	extraResources, err := request.GetExtraResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("fetching extra resources %T: %w", req, err))
		return rsp, nil
	}

	// Sort and verify min/max selected.
	// Sorting is required for determinism.
	verifiedExtras, err := verifyAndSortExtras(in, extraResources)
	if err != nil {
		return nil, errors.Wrapf(err, "sorting and verifying results")
	}

	// For now cheaply convert to JSON for serializing.
	//
	// TODO(reedjosh): look into resources.AsStruct or simlar since unsturctured k8s objects are already almost json.
	//    structpb.NewList(v []interface{}) should create an array like.
	//    Combining this and similar structures from the structpb lib should should be done to create
	//    a map[string][object] container into which the found extra resources can be dumped.
	//
	//    The found extra resources should then be directly marhsal-able via:
	//    obj := &unstructured.Unstructured{}
	//    obj.MarshalJSON()
	b, err := json.Marshal(verifiedExtras)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("cannot marshal %T: %w", verifiedExtras, err))
		return rsp, nil
	}
	s := &structpb.Struct{}
	err = protojson.Unmarshal(b, s)
	if err != nil {
		response.Fatal(rsp, errors.Errorf("cannot unmarshal %T into %T: %w", extraResources, s, err))
		return rsp, nil
	}
	response.SetContextKey(rsp, FunctionContextKeyExtraResources, structpb.NewStructValue(s))

	return rsp, nil
}

// Build requirements takes input and outputs an array of external resoruce requirements to request
// from Crossplane's external resource API.
func buildRequirements(in *v1beta1.Input, xr *resource.Composite) (*fnv1beta1.Requirements, error) {
	extraResources := make(map[string]*fnv1beta1.ResourceSelector, len(in.Spec.ExtraResources))
	for _, extraResource := range in.Spec.ExtraResources {
		extraResName := extraResource.Into
		switch extraResource.Type {
		case v1beta1.ResourceSourceTypeReference, "":
			extraResources[extraResName] = &fnv1beta1.ResourceSelector{
				ApiVersion: extraResource.APIVersion,
				Kind:       extraResource.Kind,
				Match: &fnv1beta1.ResourceSelector_MatchName{
					MatchName: extraResource.Ref.Name,
				},
			}
		case v1beta1.ResourceSourceTypeSelector:
			matchLabels := map[string]string{}
			for _, selector := range extraResource.Selector.MatchLabels {
				switch selector.GetType() {
				case v1beta1.ResourceSourceSelectorLabelMatcherTypeValue:
					// TODO validate value not to be nil
					matchLabels[selector.Key] = *selector.Value
				case v1beta1.ResourceSourceSelectorLabelMatcherTypeFromCompositeFieldPath:
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
			extraResources[extraResName] = &fnv1beta1.ResourceSelector{
				ApiVersion: extraResource.APIVersion,
				Kind:       extraResource.Kind,
				Match: &fnv1beta1.ResourceSelector_MatchLabels{
					MatchLabels: &fnv1beta1.MatchLabels{Labels: matchLabels},
				},
			}
		}
	}
	return &fnv1beta1.Requirements{ExtraResources: extraResources}, nil
}

// Verify Min/Max and sort extra resources by field path within a single kind.
func verifyAndSortExtras(in *v1beta1.Input, extraResources map[string][]resource.Extra, //nolint:gocyclo // TODO(reedjosh): refactor
) (cleanedExtras map[string][]unstructured.Unstructured, err error) {
	cleanedExtras = make(map[string][]unstructured.Unstructured)
	for _, extraResource := range in.Spec.ExtraResources {
		extraResName := extraResource.Into
		resources, ok := extraResources[extraResName]
		if !ok {
			return nil, errors.Errorf("cannot find expected extra resource %q", extraResName)
		}
		switch extraResource.GetType() {
		case v1beta1.ResourceSourceTypeReference:
			if len(resources) == 0 && in.Spec.Policy.IsResolutionPolicyOptional() {
				continue
			}
			if len(resources) > 1 {
				return nil, errors.Errorf("expected exactly one extra resource %q, got %d", extraResName, len(resources))
			}
			cleanedExtras[extraResName] = append(cleanedExtras[extraResName], *resources[0].Resource)

		case v1beta1.ResourceSourceTypeSelector:
			selector := extraResource.Selector
			if selector.MinMatch != nil && len(resources) < int(*selector.MinMatch) {
				return nil, errors.Errorf("expected at least %d extra resources %q, got %d", *selector.MinMatch, extraResName, len(resources))
			}
			if err := sortExtrasByFieldPath(resources, selector.GetSortByFieldPath()); err != nil {
				return nil, err
			}
			if selector.MaxMatch != nil && len(resources) > int(*selector.MaxMatch) {
				resources = resources[:*selector.MaxMatch]
			}
			for _, r := range resources {
				cleanedExtras[extraResName] = append(cleanedExtras[extraResName], *r.Resource)
			}
		}
	}
	return cleanedExtras, nil
}

// Sort extra resources by field path within a single kind.
func sortExtrasByFieldPath(extras []resource.Extra, path string) error { //nolint:gocyclo // TODO(phisco): refactor
	if path == "" {
		return errors.New("cannot sort by empty field path")
	}
	p := make([]struct {
		ec  resource.Extra
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
