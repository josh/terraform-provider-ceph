package main

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCephFSDirectoryPathValidator(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{"/a", true},
		{"/volumes/x/y", true},
		{"/", false},
		{"", false},
		{"a", false},
		{"/a/", false},
		{"//a", false},
		{"/a//b", false},
		{"/a/./b", false},
		{"/a/../b", false},
		{"/..", false},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("path"),
				ConfigValue: types.StringValue(tc.value),
			}
			resp := &validator.StringResponse{}
			cephFSDirectoryPath().ValidateString(context.Background(), req, resp)

			if tc.valid && resp.Diagnostics.HasError() {
				t.Errorf("expected %q to be valid, got: %v", tc.value, resp.Diagnostics)
			}
			if !tc.valid && !resp.Diagnostics.HasError() {
				t.Errorf("expected %q to be invalid", tc.value)
			}
		})
	}
}
