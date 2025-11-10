package main

import (
	"context"
	"fmt"

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

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
