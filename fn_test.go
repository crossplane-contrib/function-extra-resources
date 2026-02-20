package main

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/crossplane-contrib/function-extra-resources/input/v1beta1"
)

func TestRunFunction(t *testing.T) {
	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1.RunFunctionResponse
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"RequestExtraResources": {
			reason: "The Function should request ExtraResources",
			args: args{
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "hello"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.crossplane.io/v1alpha1",
								"kind": "XR",
								"metadata": {
									"name": "my-xr"
								},
								"spec": {
									"existingEnvSelectorLabel": "someMoreBar",
									"existingBazLabel": "someMoreBar"
								}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "extra-resources.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"spec": {
							"extraResources": [
								{	
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"type": "Reference",
									"into": "obj-0",
									"ref": {	
										"name": "my-env-config"
									}
								},
								{	
									"type": "Reference",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"into": "obj-1",
									"ref": {	
										"name": "my-second-env-config"
									}
								},
								{
									"type": "Selector",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"into": "obj-2",
									"selector": {
										"matchLabels": [
											{
												"type": "Value",
												"key": "foo",
												"value": "bar"
											}
										]
									}
								},
								{
									"type": "Selector",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"into": "obj-3",
									"selector": {
										"matchLabels": [
											{
												"key": "someMoreFoo",
												"valueFromFieldPath": "spec.missingEnvSelectorLabel",
												"fromFieldPathPolicy": "Optional"
											}
										]
									}
								},
								{
									"type": "Selector",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"into": "obj-4",
									"selector": {
										"matchLabels": [
											{
												"key": "someMoreFoo",
												"valueFromFieldPath": "spec.existingEnvSelectorLabel",
												"fromFieldPathPolicy": "Required"
											}
										]
									}
								},
								{
									"type": "Reference",
									"kind": "Foo",
									"apiVersion": "test.crossplane.io/v1alpha1",
									"namespace": "my-namespace",
									"into": "obj-5",
									"ref": {
										"name": "my-foo"
									}
								},
								{
									"type": "Selector",
									"kind": "Bar",
									"apiVersion": "test.crossplane.io/v1alpha1",
									"namespace": "my-namespace",
									"into": "obj-6",
									"selector": {
										"matchLabels": [
											{
												"key": "someMoreBar",
												"valueFromFieldPath": "spec.existingBazLabel",
												"fromFieldPathPolicy": "Required"
											}
										]
									}
								}
							]
						}
					}`),
				},
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1.Result{},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"obj-0": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-env-config",
								},
							},
							"obj-1": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-second-env-config",
								},
							},
							"obj-2": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchLabels{
									MatchLabels: &fnv1.MatchLabels{
										Labels: map[string]string{
											"foo": "bar",
										},
									},
								},
							},
							//
							// environment-config-3 is not requested because it was optional
							//
							"obj-4": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchLabels{
									MatchLabels: &fnv1.MatchLabels{
										Labels: map[string]string{
											"someMoreFoo": "someMoreBar",
										},
									},
								},
							},
							"obj-5": {
								ApiVersion: "test.crossplane.io/v1alpha1",
								Kind:       "Foo",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-foo",
								},
								Namespace: ptr.To("my-namespace"),
							},
							"obj-6": {
								ApiVersion: "test.crossplane.io/v1alpha1",
								Kind:       "Bar",
								Match: &fnv1.ResourceSelector_MatchLabels{
									MatchLabels: &fnv1.MatchLabels{
										Labels: map[string]string{
											"someMoreBar": "someMoreBar",
										},
									},
								},
								Namespace: ptr.To("my-namespace"),
							},
						},
					},
				},
			},
		},
		"RequestEnvironmentConfigsFound": {
			reason: "The Function should request the necessary EnvironmentConfigs even if they are already present in the request",
			args: args{
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "hello"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.crossplane.io/v1alpha1",
								"kind": "XR",
								"metadata": {
									"name": "my-xr"
								},
								"spec": {
									"existingEnvSelectorLabel": "someMoreBar"
								}
							}`),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"obj-0": {
							Items: []*fnv1.Resource{
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-env-config"
									},
									"data": {
										"firstKey": "firstVal",
										"secondKey": "secondVal"
									}
								}`),
								},
							},
						},
						"obj-1": {
							Items: []*fnv1.Resource{
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-second-env-config"
									},
									"data": {
										"secondKey": "secondVal-ok",
										"thirdKey": "thirdVal"
									}
								}`),
								},
							},
						},
						"obj-2": {
							Items: []*fnv1.Resource{
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-third-env-config-b"
									},
									"data": {
										"fourthKey": "fourthVal-b"
									}
								}`),
								},
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-third-env-config-a"
									},
									"data": {
										"fourthKey": "fourthVal-a"
									}
								}`),
								},
							},
						},
						"obj-3": {
							Items: []*fnv1.Resource{
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-third-env-config"
									},
									"data": {
										"fifthKey": "fifthVal"
									}
								}`),
								},
							},
						},
						"obj-4": {
							Items: []*fnv1.Resource{
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-fourth-env-config"
									},
									"data": {
										"sixthKey": "sixthVal"
									}
								}`),
								},
							},
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "extra-resources.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"spec": {
							"extraResources": [
								{
									"type": "Reference",
									"into": "obj-0",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"ref": {
										"name": "my-env-config"
									}
								},
								{
									"type": "Reference",
									"into": "obj-1",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"ref": {
										"name": "my-second-env-config"
									}
								},
								{
									"type": "Selector",
									"into": "obj-2",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"selector": {
										"matchLabels": [
											{
												"type": "Value",
												"key": "foo",
												"value": "bar"
											}
										]
									}
								},
								{
									"type": "Selector",
									"into": "obj-3",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"selector": {
										"matchLabels": [
											{
												"key": "someMoreFoo",
												"valueFromFieldPath": "spec.missingEnvSelectorLabel",
												"fromFieldPathPolicy": "Optional"
											}
										]
									}
								},
								{
									"type": "Selector",
									"into": "obj-4",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"selector": {
										"matchLabels": [
											{
												"key": "someMoreFoo",
												"valueFromFieldPath": "spec.existingEnvSelectorLabel",
												"fromFieldPathPolicy": "Required"
											}
										]
									}
								}
							]
						}
					}`),
				},
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1.Result{},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"obj-0": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-env-config",
								},
							},
							"obj-1": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-second-env-config",
								},
							},
							"obj-2": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchLabels{
									MatchLabels: &fnv1.MatchLabels{
										Labels: map[string]string{
											"foo": "bar",
										},
									},
								},
							},
							// environment-config-3 is not requested because it was optional
							"obj-4": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchLabels{
									MatchLabels: &fnv1.MatchLabels{
										Labels: map[string]string{
											"someMoreFoo": "someMoreBar",
										},
									},
								},
							},
						},
					},
					Context: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							v1beta1.FunctionContextKeyExtraResources: structpb.NewStructValue(resource.MustStructJSON(`{
									"obj-0": [
        							    {
        							        "apiVersion": "apiextensions.crossplane.io/v1beta1",
        							        "data": {
        							            "firstKey": "firstVal",
        							            "secondKey": "secondVal"
        							        },
        							        "kind": "EnvironmentConfig",
        							        "metadata": {
        							            "name": "my-env-config"
        							        }
        							    }
        							],
        							"obj-1": [
        							    {
        							        "apiVersion": "apiextensions.crossplane.io/v1beta1",
        							        "data": {
        							            "secondKey": "secondVal-ok",
        							            "thirdKey": "thirdVal"
        							        },
        							        "kind": "EnvironmentConfig",
        							        "metadata": {
        							            "name": "my-second-env-config"
        							        }
        							    }
        							],
        							"obj-2": [
        							    {
        							        "apiVersion": "apiextensions.crossplane.io/v1beta1",
        							        "data": {
        							            "fourthKey": "fourthVal-a"
        							        },
        							        "kind": "EnvironmentConfig",
        							        "metadata": {
        							            "name": "my-third-env-config-a"
        							        }
        							    },
        							    {
        							        "apiVersion": "apiextensions.crossplane.io/v1beta1",
        							        "data": {
        							            "fourthKey": "fourthVal-b"
        							        },
        							        "kind": "EnvironmentConfig",
        							        "metadata": {
        							            "name": "my-third-env-config-b"
        							        }
        							    }
        							],
        							"obj-3": [
        							    {
        							        "apiVersion": "apiextensions.crossplane.io/v1beta1",
        							        "data": {
        							            "fifthKey": "fifthVal"
        							        },
        							        "kind": "EnvironmentConfig",
        							        "metadata": {
        							            "name": "my-third-env-config"
        							        }
        							    }
        							],
        							"obj-4": [
        							    {
        							        "apiVersion": "apiextensions.crossplane.io/v1beta1",
        							        "data": {
        							            "sixthKey": "sixthVal"
        							        },
        							        "kind": "EnvironmentConfig",
        							        "metadata": {
        							            "name": "my-fourth-env-config"
        							        }
        							    }
        							]
							}`)),
						},
					},
				},
			},
		},
		"RequestEnvironmentConfigsNotFoundRequired": {
			reason: "The Function should return fatal if a required EnvironmentConfig is not found",
			args: args{
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "hello"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.crossplane.io/v1alpha1",
								"kind": "XR",
								"metadata": {
									"name": "my-xr"
								}
							}`),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"environment-config-0": {
							Items: []*fnv1.Resource{},
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "extra-resources.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"spec": {
							"extraResources": [
								{	
									"type": "Reference",
									"into": "obj-0",
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"ref": {
										"name": "my-env-config"
									}
								}
							]
						}
					}`),
				},
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1.Result{
						{
							Severity: fnv1.Severity_SEVERITY_FATAL,
							Target:   fnv1.Target_TARGET_COMPOSITE.Enum(),
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"obj-0": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-env-config",
								},
							},
						},
					},
				},
			},
		},
		"CustomContextKey": {
			reason: "The Function should put resolved extra resources into custom context key when specified.",
			args: args{
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "hello"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.crossplane.io/v1alpha1",
								"kind": "XR",
								"metadata": {
									"name": "my-xr"
								}
							}`),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"obj-0": {
							Items: []*fnv1.Resource{
								{
									Resource: resource.MustStructJSON(`{
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"kind": "EnvironmentConfig",
									"metadata": {
										"name": "my-env-config"
									}
								}`),
								},
							},
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "extra-resources.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"spec": {
							"context": {
								"key": "apiextensions.crossplane.io/environment"
							},
							"extraResources": [
								{
									"kind": "EnvironmentConfig",
									"apiVersion": "apiextensions.crossplane.io/v1beta1",
									"type": "Reference",
									"into": "obj-0",
									"ref": {	
										"name": "my-env-config"
									}
								}
							]
						}
					}`),
				},
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1.Result{},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"obj-0": {
								ApiVersion: "apiextensions.crossplane.io/v1beta1",
								Kind:       "EnvironmentConfig",
								Match: &fnv1.ResourceSelector_MatchName{
									MatchName: "my-env-config",
								},
							},
						},
					},
					Context: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"apiextensions.crossplane.io/environment": structpb.NewStructValue(resource.MustStructJSON(`{
								"obj-0": [
									{
										"apiVersion": "apiextensions.crossplane.io/v1beta1",
										"kind": "EnvironmentConfig",
										"metadata": {
											"name": "my-env-config"
										}
									}
								]
							}`)),
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			diff := cmp.Diff(tc.want.rsp, rsp, cmpopts.AcyclicTransformer("toJsonWithoutResultMessages", func(r *fnv1.RunFunctionResponse) []byte {
				// We don't care about messages.
				// cmptopts.IgnoreField wasn't working with protocmp.Transform
				// We can't split this to another transformer as
				// transformers are applied not in order but as soon as they
				// match the type, which are walked from the root (RunFunctionResponse).
				for _, result := range r.GetResults() {
					result.Message = ""
				}
				out, err := protojson.Marshal(r)
				if err != nil {
					t.Fatalf("cannot marshal %T to JSON: %s", r, err)
				}
				return out
			}))
			if diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}

func resourceWithFieldPathValue(path string, value any) resource.Required {
	u := unstructured.Unstructured{
		Object: map[string]any{},
	}
	err := fieldpath.Pave(u.Object).SetValue(path, value)
	if err != nil {
		panic(err)
	}
	return resource.Required{
		Resource: &u,
	}
}

func TestSortExtrasByFieldPath(t *testing.T) {
	type args struct {
		extras []resource.Required
		path   string
	}
	type want struct {
		extras []resource.Required
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SortByString": {
			reason: "The Function should sort the Extras by the string value at the specified field path",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.name", "c"),
					resourceWithFieldPathValue("metadata.name", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
				},
				path: "metadata.name",
			},
			want: want{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.name", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
					resourceWithFieldPathValue("metadata.name", "c"),
				},
			},
		},
		"SortByInt": {
			reason: "The Function should sort the Extras by the int value at the specified field path",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("data.someInt", 3),
					resourceWithFieldPathValue("data.someInt", 1),
					resourceWithFieldPathValue("data.someInt", 2),
				},
				path: "data.someInt",
			},
			want: want{
				extras: []resource.Required{
					resourceWithFieldPathValue("data.someInt", 1),
					resourceWithFieldPathValue("data.someInt", 2),
					resourceWithFieldPathValue("data.someInt", 3),
				},
			},
		},
		"SortByFloat": {
			reason: "The Function should sort the Extras by the float value at the specified field path",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("data.someFloat", 1.3),
					resourceWithFieldPathValue("data.someFloat", 1.1),
					resourceWithFieldPathValue("data.someFloat", 1.2),
					resourceWithFieldPathValue("data.someFloat", 1.4),
				},
				path: "data.someFloat",
			},
			want: want{
				extras: []resource.Required{
					resourceWithFieldPathValue("data.someFloat", 1.1),
					resourceWithFieldPathValue("data.someFloat", 1.2),
					resourceWithFieldPathValue("data.someFloat", 1.3),
					resourceWithFieldPathValue("data.someFloat", 1.4),
				},
			},
		},
		"InconsistentTypeSortByInt": {
			reason: "The Function should sort the Extras by the int value at the specified field path",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("data.someInt", 3),
					resourceWithFieldPathValue("data.someInt", 1),
					resourceWithFieldPathValue("data.someInt", "2"),
				},
				path: "data.someInt",
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
		"EmptyPath": {
			reason: "The Function should return an error if the path is empty",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.name", "c"),
					resourceWithFieldPathValue("metadata.name", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
				},
				path: "",
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
		"InvalidPathAll": {
			reason: "The Function should return no error if the path is invalid for all resources",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.name", "c"),
					resourceWithFieldPathValue("metadata.name", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
				},
				path: "metadata.invalid",
			},
			want: want{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.name", "c"),
					resourceWithFieldPathValue("metadata.name", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
				},
			},
		},
		"InvalidPathSome": {
			reason: "The Function should return no error if the path is invalid for some resources, just use the rest of the resources zero value",
			args: args{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.name", "c"),
					resourceWithFieldPathValue("metadata.invalid", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
				},
				path: "metadata.name",
			},
			want: want{
				extras: []resource.Required{
					resourceWithFieldPathValue("metadata.invalid", "a"),
					resourceWithFieldPathValue("metadata.name", "b"),
					resourceWithFieldPathValue("metadata.name", "c"),
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := sortExtrasByFieldPath(tc.args.extras, tc.args.path)
			if diff := cmp.Diff(tc.want.err, got, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\n(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if tc.want.err != nil {
				return
			}
			if diff := cmp.Diff(tc.want.extras, tc.args.extras); diff != "" {
				t.Errorf("%s\n(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
