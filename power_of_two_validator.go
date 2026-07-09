package main

import (
	"context"
	"fmt"
	"math/bits"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type powerOfTwoValidator struct{}

func (v powerOfTwoValidator) Description(ctx context.Context) string {
	return "value must be a power of two"
}

func (v powerOfTwoValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v powerOfTwoValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueInt64()
	if value <= 0 || bits.OnesCount64(uint64(value)) != 1 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Attribute Value",
			fmt.Sprintf("Attribute %s must be a power of two, got: %d", req.Path, value),
		)
	}
}

func powerOfTwo() validator.Int64 {
	return powerOfTwoValidator{}
}
