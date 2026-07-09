package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &CrushRuleResource{}
	_ resource.ResourceWithImportState = &CrushRuleResource{}
	_ resource.ResourceWithModifyPlan  = &CrushRuleResource{}
)

func newCrushRuleResource() resource.Resource {
	return &CrushRuleResource{}
}

type CrushRuleResource struct {
	client *restapi.Client
}

type CrushRuleResourceModel struct {
	Name          types.String `tfsdk:"name"`
	PoolType      types.String `tfsdk:"pool_type"`
	FailureDomain types.String `tfsdk:"failure_domain"`
	DeviceClass   types.String `tfsdk:"device_class"`
	Profile       types.String `tfsdk:"profile"`
	Root          types.String `tfsdk:"root"`
	RuleID        types.Int64  `tfsdk:"rule_id"`
	Ruleset       types.Int64  `tfsdk:"ruleset"`
	MinSize       types.Int64  `tfsdk:"min_size"`
	MaxSize       types.Int64  `tfsdk:"max_size"`
	Steps         types.List   `tfsdk:"steps"`
}

func (r *CrushRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_crush_rule"
}

func (r *CrushRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a Ceph CRUSH rule. CRUSH rules are immutable in Ceph, so any changes to the rule's attributes will trigger resource replacement.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CRUSH rule. This is the unique identifier for the rule.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"pool_type": resourceSchema.StringAttribute{
				MarkdownDescription: "The type of pool this rule is for. Must be either 'replicated' or 'erasure'. Defaults to 'replicated'.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("replicated"),
				Validators: []validator.String{
					stringvalidator.OneOf("replicated", "erasure"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"failure_domain": resourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH failure domain for placement (e.g., 'host', 'rack', 'osd'). Determines how replicas are distributed across the cluster.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"device_class": resourceSchema.StringAttribute{
				MarkdownDescription: "Optional device class constraint (e.g., 'ssd', 'hdd'). Restricts the rule to use only OSDs of this device class.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"profile": resourceSchema.StringAttribute{
				MarkdownDescription: "The erasure code profile name. Required when pool_type is 'erasure', ignored for replicated pools.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"root": resourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH root for placement. Defaults to 'default' if not specified.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("default"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule_id": resourceSchema.Int64Attribute{
				MarkdownDescription: "The numeric ID of the CRUSH rule (computed by Ceph).",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"ruleset": resourceSchema.Int64Attribute{
				MarkdownDescription: "The ruleset number (computed by Ceph).",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"min_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "Minimum number of replicas or chunks (computed by Ceph).",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"max_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "Maximum number of replicas or chunks (computed by Ceph).",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"steps": resourceSchema.ListNestedAttribute{
				MarkdownDescription: "Detailed CRUSH rule steps in execution order.",
				Computed:            true,
				NestedObject: resourceSchema.NestedAttributeObject{
					Attributes: map[string]resourceSchema.Attribute{
						"op": resourceSchema.StringAttribute{
							MarkdownDescription: "CRUSH step opcode (e.g., 'take', 'chooseleaf').",
							Computed:            true,
						},
						"num": resourceSchema.Int64Attribute{
							MarkdownDescription: "Optional numeric argument for the step.",
							Computed:            true,
						},
						"type": resourceSchema.StringAttribute{
							MarkdownDescription: "CRUSH bucket type referenced by the step.",
							Computed:            true,
						},
						"item": resourceSchema.Int64Attribute{
							MarkdownDescription: "CRUSH bucket or ID targeted by the step, when applicable.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (r *CrushRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *CrushRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CrushRuleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.PoolType.ValueString() == "erasure" && !data.Profile.IsNull() && !data.Profile.IsUnknown() {
		profile, err := r.client.GetErasureCodeProfile(ctx, data.Profile.ValueString())
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Unable to Validate Erasure Code Profile",
				fmt.Sprintf("Could not read erasure code profile '%s' to validate failure_domain: %s", data.Profile.ValueString(), err),
			)
		} else if profile.CrushFailureDomain != data.FailureDomain.ValueString() {
			resp.Diagnostics.AddError(
				"Failure Domain Mismatch",
				fmt.Sprintf(
					"The crush rule's failure_domain (%q) must match the erasure code profile's crush_failure_domain (%q). "+
						"Ceph ignores the crush rule's failure_domain for erasure pools, using the profile's setting instead.",
					data.FailureDomain.ValueString(),
					profile.CrushFailureDomain,
				),
			)
			return
		}
	}

	createReq := restapi.CrushRuleCreateRequest{
		Name:          data.Name.ValueString(),
		PoolType:      data.PoolType.ValueString(),
		FailureDomain: data.FailureDomain.ValueString(),
	}

	if !data.DeviceClass.IsNull() && !data.DeviceClass.IsUnknown() {
		val := data.DeviceClass.ValueString()
		createReq.DeviceClass = &val
	}

	if !data.Profile.IsNull() && !data.Profile.IsUnknown() {
		val := data.Profile.ValueString()
		createReq.Profile = &val
	}

	if !data.Root.IsNull() && !data.Root.IsUnknown() {
		val := data.Root.ValueString()
		createReq.Root = &val
	}

	err := r.client.CreateCrushRule(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create CRUSH rule '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	rule, err := r.client.GetCrushRule(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CRUSH rule '%s' after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	if diags := r.updateModelFromAPI(&data, rule); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CrushRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CrushRuleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetCrushRule(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CRUSH rule '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	if diags := r.updateModelFromAPI(&data, rule); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CrushRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"CRUSH rules are immutable in Ceph and cannot be updated. Any changes require replacing the resource.",
	)
}

func (r *CrushRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CrushRuleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteCrushRule(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete CRUSH rule '%s': %s. Note that CRUSH rules cannot be deleted if they are in use by any pools.", data.Name.ValueString(), err),
		)
		return
	}
}

func (r *CrushRuleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Only guard an in-place replacement (same name, changed topology): a plain
	// destroy or a rename lets Terraform repoint/remove the referencing pool
	// first, so those are left to apply. A same-name replace cannot, since Ceph
	// refuses to delete a rule while a pool uses it.
	if r.client == nil || req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state CrushRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || plan.Name.ValueString() != state.Name.ValueString() {
		return
	}
	if plan.PoolType.Equal(state.PoolType) &&
		plan.FailureDomain.Equal(state.FailureDomain) &&
		plan.DeviceClass.Equal(state.DeviceClass) &&
		plan.Profile.Equal(state.Profile) &&
		plan.Root.Equal(state.Root) {
		return
	}

	name := state.Name.ValueString()
	pools, err := r.client.ListPools(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to list pools to check whether CRUSH rule '%s' is in use: %s", name, err),
		)
		return
	}

	var inUse []string
	for _, pool := range pools {
		if pool.CrushRule == name {
			inUse = append(inUse, pool.PoolName)
		}
	}

	if len(inUse) > 0 {
		resp.Diagnostics.AddError(
			"CRUSH Rule In Use",
			fmt.Sprintf(
				"CRUSH rule %q is in use by pool(s): %s. Replacing it in place will fail because Ceph cannot delete a rule while a pool references it. Rename the rule (with lifecycle create_before_destroy) so the pool repoints first, or remove the pool.",
				name, strings.Join(inUse, ", "),
			),
		)
	}
}

func (r *CrushRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	ruleName := req.ID

	rule, err := r.client.GetCrushRule(ctx, ruleName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.Diagnostics.AddError(
				"CRUSH Rule Not Found",
				fmt.Sprintf("CRUSH rule '%s' does not exist in Ceph. Verify the rule name is correct.", ruleName),
			)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read CRUSH rule '%s' during import: %s", ruleName, err),
		)
		return
	}

	var data CrushRuleResourceModel
	data.Name = types.StringValue(ruleName)

	if diags := r.updateModelFromAPI(&data, rule); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// failure_domain, root, and device_class are not part of the rule dump
	// proper, but can be recovered from its steps; without them the first
	// plan after import would force a replacement. The erasure profile is
	// not recoverable and must be set in config to match.
	for _, step := range rule.Steps {
		switch {
		case step.Op == "take" && step.ItemName != "":
			root, deviceClass, hasClass := strings.Cut(step.ItemName, "~")
			data.Root = types.StringValue(root)
			if hasClass {
				data.DeviceClass = types.StringValue(deviceClass)
			}
		case strings.HasPrefix(step.Op, "choose") && step.Type != "":
			if data.FailureDomain.IsNull() {
				data.FailureDomain = types.StringValue(step.Type)
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CrushRuleResource) updateModelFromAPI(data *CrushRuleResourceModel, rule *restapi.CrushRule) diag.Diagnostics {
	var diags diag.Diagnostics

	data.RuleID = types.Int64Value(int64(rule.RuleID))
	data.Ruleset = types.Int64Value(int64(rule.Ruleset))
	data.MinSize = types.Int64Value(int64(rule.MinSize))
	data.MaxSize = types.Int64Value(int64(rule.MaxSize))

	switch rule.Type {
	case 1, 4:
		data.PoolType = types.StringValue("replicated")
	case 3, 5:
		data.PoolType = types.StringValue("erasure")
	default:
		data.PoolType = types.StringUnknown()
	}

	stepsObjects := make([]attr.Value, 0, len(rule.Steps))
	for _, step := range rule.Steps {
		stepAttrs := map[string]attr.Value{
			"op":   types.StringValue(step.Op),
			"type": types.StringValue(step.Type),
		}

		if step.Num != nil {
			stepAttrs["num"] = types.Int64Value(int64(*step.Num))
		} else {
			stepAttrs["num"] = types.Int64Null()
		}

		if step.Item != nil {
			stepAttrs["item"] = types.Int64Value(int64(*step.Item))
		} else {
			stepAttrs["item"] = types.Int64Null()
		}

		stepObj, stepDiags := types.ObjectValue(
			map[string]attr.Type{
				"op":   types.StringType,
				"num":  types.Int64Type,
				"type": types.StringType,
				"item": types.Int64Type,
			},
			stepAttrs,
		)
		diags.Append(stepDiags...)
		if diags.HasError() {
			return diags
		}

		stepsObjects = append(stepsObjects, stepObj)
	}

	stepsValue, stepDiags := types.ListValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"op":   types.StringType,
				"num":  types.Int64Type,
				"type": types.StringType,
				"item": types.Int64Type,
			},
		},
		stepsObjects,
	)
	diags.Append(stepDiags...)
	if diags.HasError() {
		return diags
	}
	data.Steps = stepsValue

	return diags
}
