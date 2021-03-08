// +build invoke_terraform

/*
Copyright 2020 The Crossplane Authors.

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

// These tests invoke the terraform binary. They require network access in
// order to download providers, and will thus not be run by default.
package terraform

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/test"
)

// Terraform binary to invoke.
var tfBinaryPath = func() string {
	if bin, ok := os.LookupEnv("TF_BINARY"); ok {
		return bin
	}
	return "terraform"
}()

// Terraform test data. We need a fully qualified path because paths are
// relative to the Terraform binary's working directory, not this test file.
var tfTestDataPath = func() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "testdata")
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		reason string
		module string
		ctx    context.Context
		want   error
	}{
		"ValidModule": {
			reason: "We should not return an error if the module is valid.",
			module: "testdata/validmodule",
			ctx:    context.Background(),
			want:   nil,
		},
		"InvalidModule": {
			reason: "We should return an error if the module is invalid.",
			module: "testdata/invalidmodule",
			ctx:    context.Background(),
			want:   errors.Errorf(errFmtInvalidConfig, 1),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Validation is a read-only operation, so we operate directly on
			// our test data instead of creating a temporary directory.
			tf := Harness{Path: tfBinaryPath, Dir: tc.module}
			got := tf.Validate(tc.ctx)

			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ntf.Validate(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestWorkspace(t *testing.T) {
	type args struct {
		ctx  context.Context
		name string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   error
	}{
		"SuccessfulSelect": {
			reason: "It should be possible to select the default workspace, which always exists.",
			args: args{
				ctx:  context.Background(),
				name: "default",
			},
			want: nil,
		},
		"SuccessfulNew": {
			reason: "It should be possible to create a new workspace.",
			args: args{
				ctx:  context.Background(),
				name: "cool",
			},
			want: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "provider-terraform-test")
			if err != nil {
				t.Fatalf("Cannot create temporary directory: %v", err)
			}
			defer os.RemoveAll(dir)

			tf := Harness{Path: tfBinaryPath, Dir: dir}
			got := tf.Workspace(tc.args.ctx, tc.args.name)

			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ntf.Workspace(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestOutput(t *testing.T) {
	type want struct {
		outputs []Output
		err     error
	}
	cases := map[string]struct {
		reason string
		module string
		ctx    context.Context
		want   want
	}{
		"ManyOutputs": {
			reason: "We should return outputs from a module.",
			module: "testdata/outputmodule",
			ctx:    context.Background(),
			want: want{
				outputs: []Output{
					{Name: "bool", Type: "bool", value: true},
					{Name: "number", Type: "number", value: float64(42)},
					{
						Name:  "object",
						Type:  "object",
						value: map[string]interface{}{"wow": "suchobject"},
					},
					{Name: "sensitive", Sensitive: true, Type: "string", value: "very"},
					{Name: "string", Type: "string", value: "very"},
					{
						Name:  "tuple",
						Type:  "tuple",
						value: []interface{}{"a", "really", "long", "tuple"},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Reading output is a read-only operation, so we operate directly
			// on our test data instead of creating a temporary directory.
			tf := Harness{Path: tfBinaryPath, Dir: tc.module}
			got, err := tf.Output(tc.ctx)

			if diff := cmp.Diff(tc.want.outputs, got, cmp.AllowUnexported(Output{})); diff != "" {
				t.Errorf("\n%s\ntf.Output(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ntf.Output(...): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestInitApplyDestroy(t *testing.T) {
	type initArgs struct {
		ctx        context.Context
		fromModule string
	}
	type args struct {
		ctx context.Context
		o   []Option
	}
	type want struct {
		init    error
		apply   error
		destroy error
	}

	cases := map[string]struct {
		reason      string
		initArgs    initArgs
		applyArgs   args
		destroyArgs args
		want        want
	}{
		"Simple": {
			reason: "It should be possible to initialize, apply, and destroy a simple Terraform module",
			initArgs: initArgs{
				ctx:        context.Background(),
				fromModule: filepath.Join(tfTestDataPath(), "nullmodule"),
			},
			applyArgs: args{
				ctx: context.Background(),
			},
			destroyArgs: args{
				ctx: context.Background(),
			},
		},
		"WithVar": {
			reason: "It should be possible to initialize a simple Terraform module, then apply and destroy it with a supplied variable",
			initArgs: initArgs{
				ctx:        context.Background(),
				fromModule: filepath.Join(tfTestDataPath(), "nullmodule"),
			},
			applyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVar("coolness", "extreme")},
			},
			destroyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVar("coolness", "extreme")},
			},
		},
		"WithHCLVarFile": {
			reason: "It should be possible to initialize a simple Terraform module, then apply and destroy it with a supplied HCL file of variables",
			initArgs: initArgs{
				ctx:        context.Background(),
				fromModule: filepath.Join(tfTestDataPath(), "nullmodule"),
			},
			applyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVarFile([]byte(`coolness = "extreme!"`), HCL)},
			},
			destroyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVarFile([]byte(`coolness = "extreme!"`), HCL)},
			},
		},
		"WithJSONVarFile": {
			reason: "It should be possible to initialize a simple Terraform module, then apply and destroy it with a supplied JSON file of variables",
			initArgs: initArgs{
				ctx:        context.Background(),
				fromModule: filepath.Join(tfTestDataPath(), "nullmodule"),
			},
			applyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVarFile([]byte(`{"coolness":"extreme!"}`), JSON)},
			},
			destroyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVarFile([]byte(`{"coolness":"extreme!"}`), JSON)},
			},
		},
		// NOTE(negz): The goal of these error case tests is to validate that
		// any kind of error classification is happening. We don't want to test
		// too many error cases, because doing so would likely create an overly
		// tight coupling to a particular version of the terraform binary.
		"ModuleNotFound": {
			reason: "Init should return an error when asked to initialize from a module that doesn't exist",
			initArgs: initArgs{
				ctx:        context.Background(),
				fromModule: "./nonexistent",
			},
			applyArgs: args{
				ctx: context.Background(),
			},
			destroyArgs: args{
				ctx: context.Background(),
			},
			want: want{
				init:  errors.Wrap(errors.New("module not found"), errInit),
				apply: errors.Wrap(errors.New("no configuration files"), errApply),
				// Apparently destroy 'works' in this situation ¯\_(ツ)_/¯
			},
		},
		"UndeclaredVar": {
			reason: "Destroy should return an error when supplied a variable not declared by the module",
			initArgs: initArgs{
				ctx:        context.Background(),
				fromModule: filepath.Join(tfTestDataPath(), "nullmodule"),
			},
			applyArgs: args{
				ctx: context.Background(),
			},
			destroyArgs: args{
				ctx: context.Background(),
				o:   []Option{WithVar("boop", "doop!")},
			},
			want: want{
				destroy: errors.Wrap(errors.New("value for undeclared variable"), errDestroy),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "provider-terraform-test")
			if err != nil {
				t.Fatalf("Cannot create temporary directory: %v", err)
			}
			defer os.RemoveAll(dir)

			tf := Harness{Path: tfBinaryPath, Dir: dir}

			got := tf.Init(tc.initArgs.ctx, tc.initArgs.fromModule)
			if diff := cmp.Diff(tc.want.init, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ntf.Init(...): -want, +got:\n%s", tc.reason, diff)
			}

			got = tf.Apply(tc.applyArgs.ctx, tc.applyArgs.o...)
			if diff := cmp.Diff(tc.want.apply, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ntf.Apply(...): -want, +got:\n%s", tc.reason, diff)
			}

			got = tf.Destroy(tc.destroyArgs.ctx, tc.destroyArgs.o...)
			if diff := cmp.Diff(tc.want.destroy, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ntf.Destroy(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
