package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type noMgrPrefixKeysValidator struct{}

func (v noMgrPrefixKeysValidator) Description(ctx context.Context) string {
	return "ensures no map keys start with mgr/ prefix"
}

func (v noMgrPrefixKeysValidator) MarkdownDescription(ctx context.Context) string {
	return "Ensures no map keys start with `mgr/` prefix. Use `ceph_mgr_module_config` instead."
}

func (v noMgrPrefixKeysValidator) ValidateMap(ctx context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}

	for key := range req.ConfigValue.Elements() {
		if strings.HasPrefix(key, "mgr/") {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				req.Path,
				"Invalid Configuration Name",
				fmt.Sprintf("Configuration '%s' cannot be managed via ceph_config. Use ceph_mgr_module_config instead.", key),
			))
		}
	}
}

func NoMgrPrefixKeys() validator.Map {
	return noMgrPrefixKeysValidator{}
}

type noEmptyValuesValidator struct{}

func (v noEmptyValuesValidator) Description(ctx context.Context) string {
	return "ensures no map values are empty strings"
}

func (v noEmptyValuesValidator) MarkdownDescription(ctx context.Context) string {
	return "Ensures no map values are empty strings."
}

func (v noEmptyValuesValidator) ValidateMap(ctx context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}

	for key, value := range req.ConfigValue.Elements() {
		str, ok := value.(types.String)
		if !ok || str.IsUnknown() || str.IsNull() {
			continue
		}
		if str.ValueString() == "" {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				req.Path,
				"Invalid Configuration Value",
				fmt.Sprintf("Configuration '%s' cannot be an empty string. The Ceph dashboard treats an empty value as a removal; omit the key instead.", key),
			))
		}
	}
}

func NoEmptyValues() validator.Map {
	return noEmptyValuesValidator{}
}
