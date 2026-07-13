package main

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// The dashboard's directory endpoints use the configured path verbatim
// while reads normalize it, so an unnormalized path would create fine
// but never be found again.
type cephFSDirectoryPathValidator struct{}

func (v cephFSDirectoryPathValidator) Description(ctx context.Context) string {
	return "value must be an absolute, normalized directory path other than '/'"
}

func (v cephFSDirectoryPathValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v cephFSDirectoryPathValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if !strings.HasPrefix(value, "/") || value == "/" || path.Clean(value) != value {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Attribute Value",
			fmt.Sprintf("Attribute %s must be an absolute, normalized directory path: begin with '/', not be '/', and contain no trailing slash and no empty, '.' or '..' segments, got: %s", req.Path, value),
		)
	}
}

func cephFSDirectoryPath() validator.String {
	return cephFSDirectoryPathValidator{}
}
