package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &CrushRuleResource{}
	_ resource.ResourceWithImportState = &CrushRuleResource{}
)

func newCrushRuleResource() resource.Resource {
	return &CrushRuleResource{}
}

type CrushRuleResource struct {
	client *CephAPIClient
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
	Type          types.Int64  `tfsdk:"type"`
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
			"type": resourceSchema.Int64Attribute{
				MarkdownDescription: "The type code of the rule: 1 for replicated, 3 for erasure coded (computed by Ceph).",
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

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
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

	createReq := CephAPICrushRuleCreateRequest{
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
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete CRUSH rule '%s': %s. Note that CRUSH rules cannot be deleted if they are in use by any pools.", data.Name.ValueString(), err),
		)
		return
	}
}

func (r *CrushRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *CrushRuleResource) updateModelFromAPI(data *CrushRuleResourceModel, rule *CephAPICrushRule) diag.Diagnostics {
	var diags diag.Diagnostics

	data.RuleID = types.Int64Value(int64(rule.RuleID))
	data.Ruleset = types.Int64Value(int64(rule.Ruleset))
	data.Type = types.Int64Value(int64(rule.Type))
	data.MinSize = types.Int64Value(int64(rule.MinSize))
	data.MaxSize = types.Int64Value(int64(rule.MaxSize))

	stepsObjects := make([]attr.Value, 0, len(rule.Steps))
	for _, step := range rule.Steps {
		stepAttrs := map[string]attr.Value{
			"op":   types.StringValue(step.Op),
			"type": types.StringValue(step.Type),
		}

		if step.Num != 0 {
			stepAttrs["num"] = types.Int64Value(int64(step.Num))
		} else {
			stepAttrs["num"] = types.Int64Null()
		}

		if step.Item != 0 {
			stepAttrs["item"] = types.Int64Value(int64(step.Item))
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
