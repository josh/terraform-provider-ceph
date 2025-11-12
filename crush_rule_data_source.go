package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CrushRuleDataSource{}

func newCrushRuleDataSource() datasource.DataSource {
	return &CrushRuleDataSource{}
}

type CrushRuleDataSource struct {
	client *CephAPIClient
}

type CrushRuleDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	RuleID  types.Int64  `tfsdk:"rule_id"`
	Ruleset types.Int64  `tfsdk:"ruleset"`
	Type    types.Int64  `tfsdk:"type"`
	MinSize types.Int64  `tfsdk:"min_size"`
	MaxSize types.Int64  `tfsdk:"max_size"`
	Steps   types.List   `tfsdk:"steps"`
}

func (d *CrushRuleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_crush_rule"
}

func (d *CrushRuleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a CRUSH rule.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the CRUSH rule",
				Required:            true,
			},
			"rule_id": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The numeric ID of the CRUSH rule",
				Computed:            true,
			},
			"ruleset": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The ruleset number",
				Computed:            true,
			},
			"type": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The type of rule (1 = replicated, 3 = erasure coded)",
				Computed:            true,
			},
			"min_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Minimum number of replicas or chunks",
				Computed:            true,
			},
			"max_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Maximum number of replicas or chunks",
				Computed:            true,
			},
			"steps": dataSourceSchema.ListNestedAttribute{
				MarkdownDescription: "Detailed CRUSH rule steps in execution order.",
				Computed:            true,
				NestedObject: dataSourceSchema.NestedAttributeObject{
					Attributes: map[string]dataSourceSchema.Attribute{
						"op": dataSourceSchema.StringAttribute{
							MarkdownDescription: "CRUSH step opcode (e.g., 'take', 'chooseleaf').",
							Computed:            true,
						},
						"num": dataSourceSchema.Int64Attribute{
							MarkdownDescription: "Optional numeric argument for the step.",
							Computed:            true,
						},
						"type": dataSourceSchema.StringAttribute{
							MarkdownDescription: "CRUSH bucket type referenced by the step.",
							Computed:            true,
						},
						"item": dataSourceSchema.Int64Attribute{
							MarkdownDescription: "CRUSH bucket or ID targeted by the step, when applicable.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *CrushRuleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *CrushRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CrushRuleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()

	rule, err := d.client.GetCrushRule(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get CRUSH rule '%s' from Ceph API: %s", name, err),
		)
		return
	}

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
		resp.Diagnostics.Append(stepDiags...)
		if resp.Diagnostics.HasError() {
			return
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
	resp.Diagnostics.Append(stepDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Steps = stepsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
